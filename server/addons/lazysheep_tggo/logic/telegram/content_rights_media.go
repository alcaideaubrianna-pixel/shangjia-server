// Package telegram
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package telegram

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	rightsMediaSecret   = "dc7f7fbb4f36fbb43071882d4a1ae7a514996adcb21464e6988eccaa64aa3ed3"
	rightsMediaMaxBytes = 48 << 20
)

var rightsMediaHTTPClient = &http.Client{
	Timeout: 90 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

func downloadRightsMedia(ctx context.Context, rawURL string) (string, []byte, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil, fmt.Errorf("媒体地址为空")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "video/*,image/*,*/*;q=0.8")
	resp, err := rightsMediaHTTPClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", nil, fmt.Errorf("下载媒体失败，HTTP状态：%d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, rightsMediaMaxBytes+1))
	if err != nil {
		return "", nil, err
	}
	if len(body) == 0 {
		return "", nil, fmt.Errorf("媒体内容为空")
	}
	if len(body) > rightsMediaMaxBytes {
		return "", nil, fmt.Errorf("媒体文件超过 %dMB", rightsMediaMaxBytes>>20)
	}
	return rightsMediaFilename(rawURL), decodeRightsMedia(rawURL, body), nil
}

func decodeRightsMedia(rawURL string, body []byte) []byte {
	if looksLikeKnownMedia(body) {
		return body
	}
	decoded, err := decodeRightsMediaBytes(rawURL, body)
	if err != nil || len(decoded) == 0 {
		return body
	}
	return decoded
}

func decodeRightsMediaBytes(rawURL string, body []byte) ([]byte, error) {
	parts := strings.Split(rawURL, "/")
	if len(parts) < 4 || len(body) == 0 {
		return nil, fmt.Errorf("invalid media url")
	}
	key := hmac.New(sha256.New, []byte(rightsMediaSecret))
	_, _ = key.Write([]byte(strings.Join(parts[3:], "/")))
	xorKey := key.Sum(nil)
	start := int(body[0]) + 1
	if start > len(body) {
		return nil, fmt.Errorf("invalid media offset")
	}
	payload := body[start:]
	out := make([]byte, len(payload))
	for i := range payload {
		out[i] = payload[i] ^ xorKey[i%len(xorKey)]
	}
	return out, nil
}

func looksLikeKnownMedia(body []byte) bool {
	if len(body) >= 3 && body[0] == 0xff && body[1] == 0xd8 && body[2] == 0xff {
		return true
	}
	if len(body) >= 8 && body[0] == 0x89 && string(body[1:4]) == "PNG" {
		return true
	}
	if len(body) >= 12 && string(body[4:8]) == "ftyp" {
		return true
	}
	return false
}

func rightsMediaFilename(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil {
		base := path.Base(u.Path)
		if base != "." && base != "/" && strings.Contains(base, ".") {
			return sanitizeRightsMediaFilename(base)
		}
	}
	return "verify.mp4"
}

func sanitizeRightsMediaFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "verify.mp4"
	}
	return b.String()
}
