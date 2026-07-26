// Package bangchat implements the authorized BangChat browser API client.
package bangchat

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/net/proxy"
	"hotgo/internal/library/cache"
)

const (
	apiBaseURL       = "https://seats.bangchats.top/api"
	publicAPIBaseURL = "https://note.bangchat.icu/api"
)

var (
	httpClient = &http.Client{
		Transport: newTransport(""),
		Timeout:   90 * time.Second,
	}

	streamHTTPClient = &http.Client{
		Transport: newTransport(""),
	}
	clientMu sync.Mutex
)

func SetProxy(proxyRaw string) error {
	clientMu.Lock()
	defer clientMu.Unlock()
	transport, err := buildTransport(proxyRaw)
	if err != nil {
		return err
	}
	streamTransport, err := buildTransport(proxyRaw)
	if err != nil {
		return err
	}
	httpClient.Transport = transport
	streamHTTPClient.Transport = streamTransport
	return nil
}

func newTransport(proxyRaw string) *http.Transport {
	transport, _ := buildTransport(proxyRaw)
	return transport
}

func buildTransport(proxyRaw string) (*http.Transport, error) {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	proxyRaw = strings.TrimSpace(proxyRaw)
	if proxyRaw == "" {
		return transport, nil
	}
	parsed, err := url.Parse(proxyRaw)
	if err != nil {
		return nil, fmt.Errorf("BangChat proxy parse failed: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsed)
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(parsed, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("BangChat socks5 proxy init failed: %w", err)
		}
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.Dial(network, address)
		}
	default:
		return nil, fmt.Errorf("BangChat proxy only supports http, https, socks5")
	}
	return transport, nil
}

type Client struct {
	apiBase string
	secret  string
	iv      string
	pubKey  string
	priv    *ecdsa.PrivateKey
	jwt     string
}

type PullOption struct {
	URL      string
	Limit    int
	MaxPages int
	PageSize int
}

type PullResult struct {
	PairID   string
	Messages []json.RawMessage
}

type PullPage struct {
	PairID   string
	Page     int
	Messages []json.RawMessage
}

type Session struct {
	Client *Client
	PairID string
}

func Pull(ctx context.Context, opt PullOption) (*PullResult, error) {
	if opt.Limit <= 0 {
		opt.Limit = 50
	}
	session, err := OpenSession(ctx, opt.URL)
	if err != nil {
		return nil, err
	}
	messages, err := session.Client.CollectMessages(ctx, session.PairID, opt.Limit, opt.MaxPages)
	if err != nil {
		if isBangChatAuthExpiredError(err) && clearResolvedToken(ctx, opt.URL) {
			session, retryErr := OpenSession(ctx, opt.URL)
			if retryErr == nil {
				messages, retryErr = session.Client.CollectMessages(ctx, session.PairID, opt.Limit, opt.MaxPages)
			}
			if retryErr == nil {
				return &PullResult{PairID: session.PairID, Messages: messages}, nil
			}
		}
		return nil, err
	}
	return &PullResult{PairID: session.PairID, Messages: messages}, nil
}

func PullPages(ctx context.Context, opt PullOption, handle func(*PullPage) error) (pairID string, err error) {
	if handle == nil {
		return "", errors.New("pull page handler is nil")
	}
	if isPublicNoteSourceURL(opt.URL) {
		return PullPublicNotePages(ctx, opt, handle)
	}
	session, err := OpenSession(ctx, opt.URL)
	if err != nil {
		return "", err
	}
	handledAny := false
	if err = session.Client.CollectMessagePages(ctx, session.PairID, opt.Limit, opt.MaxPages, opt.PageSize, func(page int, messages []json.RawMessage) error {
		handledAny = true
		return handle(&PullPage{
			PairID:   session.PairID,
			Page:     page,
			Messages: messages,
		})
	}); err != nil {
		if !handledAny && isBangChatAuthExpiredError(err) && clearResolvedToken(ctx, opt.URL) {
			session, retryErr := OpenSession(ctx, opt.URL)
			if retryErr == nil {
				retryErr = session.Client.CollectMessagePages(ctx, session.PairID, opt.Limit, opt.MaxPages, opt.PageSize, func(page int, messages []json.RawMessage) error {
					return handle(&PullPage{
						PairID:   session.PairID,
						Page:     page,
						Messages: messages,
					})
				})
			}
			if retryErr == nil {
				return session.PairID, nil
			}
		}
		return "", err
	}
	return session.PairID, nil
}

func OpenSession(ctx context.Context, sourceURL string) (*Session, error) {
	token, err := ResolveToken(ctx, sourceURL)
	if err != nil {
		return nil, err
	}
	client, err := NewClientWithBase(ctx, apiBaseFromSourceURL(sourceURL))
	if err != nil {
		return nil, err
	}
	pairID, err := client.RegisterByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return &Session{Client: client, PairID: pairID}, nil
}

func apiBaseFromSourceURL(sourceURL string) string {
	u, err := url.Parse(strings.TrimSpace(sourceURL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return apiBaseURL
	}
	return (&url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   "/api",
	}).String()
}

func ResolveToken(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if u, err := url.Parse(input); err == nil {
		if token := u.Query().Get("token"); token != "" {
			return token, nil
		}
		if u.Scheme == "http" || u.Scheme == "https" {
			cacheKey := resolveTokenCacheKey(u)
			if cached := loadResolvedToken(ctx, cacheKey); cached != "" {
				return cached, nil
			}
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, input, nil)
			resp, err := httpClient.Do(req)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			if token := resp.Request.URL.Query().Get("token"); token != "" {
				saveResolvedToken(ctx, cacheKey, token)
				return token, nil
			}
		}
	}
	if strings.HasPrefix(input, "http") {
		return "", fmt.Errorf("token not found in %s", input)
	}
	if input == "" {
		return "", errors.New("source url is empty")
	}
	return input, nil
}

func resolveTokenCacheKey(u *url.URL) string {
	if u == nil {
		return ""
	}
	clone := *u
	query := clone.Query()
	query.Del("t")
	clone.RawQuery = query.Encode()
	return "bangchat:resolved-token:" + clone.String()
}

func loadResolvedToken(ctx context.Context, key string) string {
	if key == "" {
		return ""
	}
	defer func() {
		_ = recover()
	}()
	val, err := cache.Instance().Get(ctx, key)
	if err != nil || val.IsNil() {
		return ""
	}
	return strings.TrimSpace(val.String())
}

func saveResolvedToken(ctx context.Context, key, token string) {
	if key == "" || strings.TrimSpace(token) == "" {
		return
	}
	defer func() {
		_ = recover()
	}()
	_ = cache.Instance().Set(ctx, key, strings.TrimSpace(token), time.Hour*24)
}

func clearResolvedToken(ctx context.Context, input string) bool {
	input = strings.TrimSpace(input)
	u, err := url.Parse(input)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	key := resolveTokenCacheKey(u)
	if key == "" {
		return false
	}
	defer func() {
		_ = recover()
	}()
	_, _ = cache.Instance().Remove(ctx, key)
	return true
}

func isBangChatAuthExpiredError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "user no login") ||
		strings.Contains(text, "not login") ||
		strings.Contains(text, "unauthorized") ||
		strings.Contains(text, "401")
}

func NewClient(ctx context.Context) (*Client, error) {
	return NewClientWithBase(ctx, apiBaseURL)
}

func NewClientWithBase(ctx context.Context, apiBase string) (*Client, error) {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		apiBase = apiBaseURL
	}
	pub, err := getPublicKey(ctx, apiBase)
	if err != nil {
		return nil, err
	}
	serverPubBytes, err := base64.StdEncoding.DecodeString(pub)
	if err != nil {
		return nil, err
	}
	curve := ecdh.P256()
	serverPub, err := curve.NewPublicKey(serverPubBytes)
	if err != nil {
		return nil, err
	}
	priv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := priv.ECDH(serverPub)
	if err != nil {
		return nil, err
	}
	secret, iv := deriveSecretIV(shared)
	wallet, err := crypto.GenerateKey()
	if err != nil {
		return nil, err
	}
	return &Client{
		apiBase: apiBase,
		secret:  secret,
		iv:      iv,
		pubKey:  base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes()),
		priv:    wallet,
	}, nil
}

func (c *Client) RegisterByToken(ctx context.Context, token string) (pairID string, err error) {
	seat, err := c.signedPost(ctx, "/v1.Passport/GetSeatInfoByToken", map[string]string{"token": token})
	if err != nil {
		return "", err
	}
	var seatResp struct {
		RoomID  string `json:"roomId"`
		RoomID2 string `json:"room_id"`
		UserID  string `json:"userId"`
		UserID2 string `json:"user_id"`
		Expire  string `json:"expire"`
	}
	_ = json.Unmarshal([]byte(seat), &seatResp)

	reg, err := c.signedPost(ctx, "/v1.Passport/CasualRoomChatRegister", map[string]any{
		"token":    token,
		"uid":      firstNonEmpty(seatResp.UserID, seatResp.UserID2),
		"id":       firstNonEmpty(seatResp.RoomID, seatResp.RoomID2),
		"expire":   seatResp.Expire,
		"password": "",
	})
	if err != nil {
		return "", err
	}
	var regResp struct {
		Token struct {
			AccessToken string `json:"accessToken"`
		} `json:"token"`
		Room struct {
			RoomID string `json:"roomId"`
			ID     string `json:"id"`
		} `json:"room"`
	}
	_ = json.Unmarshal([]byte(reg), &regResp)
	c.jwt = regResp.Token.AccessToken
	roomID := firstNonZero(parseInt64(regResp.Room.RoomID), parseInt64(regResp.Room.ID), parseInt64(seatResp.RoomID), parseInt64(seatResp.RoomID2))
	return roomPair(roomID), nil
}

func (c *Client) CollectMessages(ctx context.Context, pairID string, limit int, maxPages int) ([]json.RawMessage, error) {
	all := make([]json.RawMessage, 0, limit)
	if err := c.CollectMessagePages(ctx, pairID, limit, maxPages, 0, func(_ int, messages []json.RawMessage) error {
		all = append(all, messages...)
		return nil
	}); err != nil {
		return nil, err
	}
	return all, nil
}

func (c *Client) CollectMessagePages(ctx context.Context, pairID string, limit int, maxPages int, pageSize int, handle func(page int, messages []json.RawMessage) error) error {
	if handle == nil {
		return errors.New("message page handler is nil")
	}
	if pageSize <= 0 {
		pageSize = 50
	}
	if limit > 0 && limit < pageSize {
		pageSize = limit
	}
	seen := make(map[string]struct{})
	collected := 0
	maxID := int64(0)
	for page := 1; ; page++ {
		if maxPages > 0 && page > maxPages {
			break
		}
		if limit > 0 && collected >= limit {
			break
		}
		pageLimit := pageSize
		if limit > 0 && limit-collected < pageLimit {
			pageLimit = limit - collected
		}
		pageResp, err := c.messageListPage(ctx, pairID, maxID, pageLimit)
		if err != nil {
			if isTransientHTTPError(err) && collected > 0 {
				break
			}
			return err
		}
		var parsed struct {
			Code    int               `json:"code"`
			Message string            `json:"message"`
			List    []json.RawMessage `json:"list"`
			Data    struct {
				List []json.RawMessage `json:"list"`
			} `json:"data"`
		}
		if !strings.HasPrefix(strings.TrimSpace(pageResp), "{") && !strings.HasPrefix(strings.TrimSpace(pageResp), "[") {
			return fmt.Errorf("message page returned non-json response: %s", abbreviate(pageResp, 80))
		}
		if err := json.Unmarshal([]byte(pageResp), &parsed); err != nil {
			return fmt.Errorf("parse message page failed: %w: %s", err, abbreviate(pageResp, 200))
		}
		list := parsed.List
		if len(list) == 0 && len(parsed.Data.List) > 0 {
			list = parsed.Data.List
		}
		if parsed.Code != 0 && len(list) == 0 {
			return fmt.Errorf("message list failed: %s", parsed.Message)
		}
		if len(list) == 0 {
			break
		}
		pageMessages := make([]json.RawMessage, 0, len(list))
		for _, raw := range list {
			id := rawMessageID(raw)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			pageMessages = append(pageMessages, raw)
			collected++
			if limit > 0 && collected >= limit {
				break
			}
		}
		if len(pageMessages) > 0 {
			if err := handle(page, pageMessages); err != nil {
				return err
			}
		}
		oldestID := rawMessageID(list[len(list)-1])
		nextID := parseInt64(oldestID)
		if nextID == 0 || nextID == maxID || len(list) < pageLimit || (limit > 0 && collected >= limit) {
			break
		}
		maxID = nextID
	}
	return nil
}

func (c *Client) messageListPage(ctx context.Context, pairID string, maxID int64, pageLimit int) (string, error) {
	var lastErr error
	for _, limit := range fallbackPageLimits(pageLimit) {
		resp, err := c.signedPost(ctx, "/v1.Message/List", map[string]any{
			"pair_id":       pairID,
			"max_id":        maxID,
			"include_quote": true,
			"pager":         map[string]any{"limit": limit},
		})
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isTransientHTTPError(err) {
			break
		}
	}
	return "", lastErr
}

func fallbackPageLimits(pageLimit int) []int {
	if pageLimit <= 10 {
		return []int{pageLimit}
	}
	limits := []int{pageLimit}
	if pageLimit > 20 {
		limits = append(limits, 20)
	}
	limits = append(limits, 10)
	return limits
}

func getPublicKey(ctx context.Context, apiBase string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(apiBase, "/")+"/v1.Setting/GetPublicKey", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var parsed struct {
		PublicKey string `json:"publicKey"`
		Publickey string `json:"public_key"`
		Data      struct {
			PublicKey string `json:"public_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("public key parse failed: %w: %s", err, string(body))
	}
	pub := firstNonEmpty(parsed.PublicKey, parsed.Publickey, parsed.Data.PublicKey)
	if pub == "" {
		return "", fmt.Errorf("public key is empty: %s", string(body))
	}
	return pub, nil
}

func (c *Client) signedPost(ctx context.Context, apiPath string, payload any) (string, error) {
	return c.signedPostWithRetry(ctx, apiPath, payload, 3)
}

func (c *Client) signedPostWithRetry(ctx context.Context, apiPath string, payload any, attempts int) (string, error) {
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		body, err := c.signedPostOnce(ctx, apiPath, payload)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !isTransientHTTPError(err) || i == attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(i+1) * 800 * time.Millisecond):
		}
	}
	return "", lastErr
}

func (c *Client) signedPostOnce(ctx context.Context, apiPath string, payload any) (string, error) {
	req, random, err := c.newSignedRequest(ctx, apiPath, payload)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if dec, derr := xorBase64Decode(string(raw), []byte(random)); derr == nil && looksLikeJSON(dec) {
		return string(dec), nil
	}
	return string(raw), nil
}

func isTransientHTTPError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unexpected eof") ||
		strings.Contains(text, "connection reset") ||
		strings.Contains(text, "broken pipe") ||
		strings.Contains(text, "server closed idle connection")
}

func (c *Client) newSignedRequest(ctx context.Context, apiPath string, payload any) (*http.Request, string, error) {
	plain, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	random := randomString(32)
	encBody := xorBase64(plain, []byte(random))
	key, sign, ts, err := c.signRequest(encBody, random)
	if err != nil {
		return nil, "", err
	}
	apiBase := strings.TrimRight(c.apiBase, "/")
	if apiBase == "" {
		apiBase = apiBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+apiPath, strings.NewReader(encBody))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("token", c.jwt)
	req.Header.Set("sign", sign)
	req.Header.Set("key", key)
	req.Header.Set("c-request-time", fmt.Sprintf("%d", ts))
	req.Header.Set("X-Client-Key", c.pubKey)
	req.Header.Set("dno", randomUUID())
	req.Header.Set("isWeb", "true")
	return req, random, nil
}

func (c *Client) signRequest(encBody string, random string) (string, string, int64, error) {
	if c == nil || c.priv == nil {
		return "", "", 0, errors.New("missing wallet")
	}
	key, err := c.encryptRandom(random)
	if err != nil {
		return "", "", 0, err
	}
	ts := time.Now().Unix()
	hash := crypto.Keccak256([]byte(encBody), []byte(key), []byte(fmt.Sprintf("%d", ts)))
	sig, err := crypto.Sign(hash, c.priv)
	if err != nil {
		return "", "", 0, err
	}
	if sig[64] == 27 || sig[64] == 28 {
		sig[64] -= 27
	}
	return key, strings.ToLower(crypto.PubkeyToAddress(c.priv.PublicKey).Hex()) + fmt.Sprintf("%x", sig), ts, nil
}

func (c *Client) encryptRandom(plain string) (string, error) {
	secret, err := mustBase64(c.secret)
	if err != nil {
		return "", err
	}
	iv, err := mustBase64(c.iv)
	if err != nil {
		return "", err
	}
	ciphertext, err := aesCBCEncrypt([]byte(plain), secret, iv)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func aesCBCEncrypt(plain, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(append([]byte{}, plain...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return out, nil
}

func deriveSecretIV(shared []byte) (string, string) {
	sum := sha256.Sum256(shared)
	prefix := append([]byte("BangOS-IV"), shared...)
	ivSum := sha256.Sum256(prefix)
	return base64.StdEncoding.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(ivSum[:16])
}

func mustBase64(raw string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(raw)
}

func xorBase64(plain []byte, key []byte) string {
	out := make([]byte, len(plain))
	for i := range plain {
		out[i] = plain[i] ^ key[i%len(key)]
	}
	return base64.StdEncoding.EncodeToString(out)
}

func xorBase64Decode(enc string, key []byte) ([]byte, error) {
	enc = strings.TrimSpace(enc)
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(enc)
	}
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(enc)
	}
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(raw))
	for i := range raw {
		out[i] = raw[i] ^ key[i%len(key)]
	}
	return out, nil
}

func looksLikeJSON(raw []byte) bool {
	text := strings.TrimSpace(string(raw))
	return strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[")
}

func randomString(n int) string {
	if n <= 10 {
		n = 32
	}
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz013456789"
	r := make([]byte, n-10)
	_, _ = rand.Read(r)
	sb := strings.Builder{}
	sb.Grow(n)
	for i := range r {
		sb.WriteByte(chars[int(r[i])%len(chars)])
	}
	sb.WriteString(fmt.Sprintf("%010d", time.Now().Unix()))
	return sb.String()
}

func randomUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func firstNonZero(vals ...int64) int64 {
	for _, v := range vals {
		if v != 0 {
			return v
		}
	}
	return 0
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}

func roomPair(roomID int64) string {
	if roomID == 0 || roomID > math.MaxInt {
		return ""
	}
	return encodeBase32(0) + encodeBase32(int(roomID))
}

func encodeBase32(v int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuv"
	if v < 0 {
		return ""
	}
	if v == 0 {
		return "00000000"
	}
	out := ""
	for v > 0 {
		out = string(alphabet[v%32]) + out
		v /= 32
	}
	for len(out) < 8 {
		out = "0" + out
	}
	return out
}

func rawMessageID(raw json.RawMessage) string {
	var v struct {
		ID   string `json:"id"`
		UpID string `json:"upId"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	return firstNonEmpty(v.ID, v.UpID)
}

func abbreviate(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return text[:max] + "..."
}
