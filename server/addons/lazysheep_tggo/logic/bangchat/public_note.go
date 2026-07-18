package bangchat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const publicNotePairID = "public_note_share"

func isPublicNoteSourceURL(sourceURL string) bool {
	_, ok := publicNoteCodeFromURL(sourceURL)
	return ok
}

func PullPublicNotePages(ctx context.Context, opt PullOption, handle func(*PullPage) error) (string, error) {
	if handle == nil {
		return "", errors.New("public note page handler is nil")
	}
	code, err := ResolvePublicNoteCode(ctx, opt.URL)
	if err != nil {
		return "", err
	}
	client, err := NewClientWithBase(ctx, publicAPIBaseURL)
	if err != nil {
		return "", err
	}
	items, err := client.ListPublicNotes(ctx, code)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return publicNotePairID, nil
	}
	pageSize := opt.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if opt.Limit > 0 && opt.Limit < pageSize {
		pageSize = opt.Limit
	}
	collected := 0
	for page, start := 1, 0; start < len(items); page, start = page+1, start+pageSize {
		if opt.MaxPages > 0 && page > opt.MaxPages {
			break
		}
		if opt.Limit > 0 && collected >= opt.Limit {
			break
		}
		end := start + pageSize
		if end > len(items) {
			end = len(items)
		}
		if opt.Limit > 0 && collected+(end-start) > opt.Limit {
			end = start + (opt.Limit - collected)
		}
		messages := make([]json.RawMessage, 0, end-start)
		for _, item := range items[start:end] {
			raw, err := publicNoteToMessage(code, item)
			if err != nil {
				return "", err
			}
			messages = append(messages, raw)
			collected++
		}
		if len(messages) == 0 {
			break
		}
		if err := handle(&PullPage{PairID: publicNotePairID, Page: page, Messages: messages}); err != nil {
			return "", err
		}
	}
	return publicNotePairID, nil
}

func ResolvePublicNoteCode(ctx context.Context, sourceURL string) (string, error) {
	sourceURL = strings.TrimSpace(sourceURL)
	if code, ok := publicNoteCodeFromURL(sourceURL); ok && code != "" {
		return code, nil
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if code, ok := publicNoteCodeFromURL(resp.Request.URL.String()); ok && code != "" {
		return code, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if code := extractPublicNoteCode(string(body)); code != "" {
		return code, nil
	}
	return "", fmt.Errorf("public note code not found in %s", sourceURL)
}

func publicNoteCodeFromURL(rawURL string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u == nil {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "note.bangchat.icu" && strings.HasPrefix(strings.TrimRight(u.Path, "/"), "/note/list"):
		return strings.TrimSpace(u.Query().Get("code")), true
	case host == "album.bangchat.top" || strings.HasSuffix(host, ".butlerchat.top"):
		return "", true
	default:
		return "", false
	}
}

func extractPublicNoteCode(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	markers := []string{"note/list?code=", `code=`}
	for _, marker := range markers {
		idx := strings.Index(body, marker)
		if idx < 0 {
			continue
		}
		rest := body[idx+len(marker):]
		if code := readCodePrefix(rest); code != "" {
			return code
		}
	}
	return ""
}

func readCodePrefix(text string) string {
	text = strings.TrimLeft(text, " \t\r\n\"'")
	var b strings.Builder
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
			continue
		}
		break
	}
	return b.String()
}

type PublicNoteItem struct {
	ID          json.RawMessage `json:"id"`
	ContentID   json.RawMessage `json:"contentId"`
	UpID        json.RawMessage `json:"upId"`
	Items       []any           `json:"items"`
	CreateTime  json.RawMessage `json:"createTime"`
	UpdateTime  json.RawMessage `json:"updateTime"`
	TopTime     json.RawMessage `json:"topTime"`
	DeleteTime  json.RawMessage `json:"deleteTime"`
	Sort        json.RawMessage `json:"sort"`
	Tags        []any           `json:"tags"`
	OffShelf    bool            `json:"offShelf"`
	RoomName    string          `json:"roomName"`
	Sender      string          `json:"sender"`
	SenderDno   string          `json:"senderDno"`
	SenderUser  json.RawMessage `json:"senderUser"`
	MessageType string          `json:"type"`
}

func (c *Client) ListPublicNotes(ctx context.Context, code string) ([]PublicNoteItem, error) {
	body, err := c.signedPost(ctx, "/v1.UserNotes/ListShare", map[string]any{"code": code})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Code    int    `json:"code"`
		Status  int    `json:"status"`
		Message string `json:"message"`
		Data    struct {
			List []PublicNoteItem `json:"list"`
		} `json:"data"`
		List []PublicNoteItem `json:"list"`
	}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, fmt.Errorf("parse public note list failed: %w: %s", err, abbreviate(body, 200))
	}
	list := parsed.List
	if len(list) == 0 {
		list = parsed.Data.List
	}
	if parsed.Code != 0 && parsed.Status != 200 && len(list) == 0 {
		return nil, fmt.Errorf("public note list failed: %s", parsed.Message)
	}
	return list, nil
}

func publicNoteToMessage(code string, item PublicNoteItem) (json.RawMessage, error) {
	now := time.Now().Unix()
	upID := rawJSONInt64(item.UpID, fallbackPublicNoteID(code, item.ID, "up"))
	createTime := rawJSONInt64(item.CreateTime, now)
	updateTime := rawJSONInt64(item.UpdateTime, createTime)
	topTime := rawJSONInt64(item.TopTime, updateTime)
	items := normalizePublicNoteItems(item.Items)
	noteContent := map[string]any{
		"upId":       upID,
		"items":      items,
		"createTime": createTime,
		"updateTime": updateTime,
		"topTime":    topTime,
		"deleteTime": rawJSONInt64(item.DeleteTime, 0),
		"sort":       rawJSONInt64(item.Sort, updateTime),
		"tags":       item.Tags,
		"offShelf":   item.OffShelf,
	}
	contentBytes, err := json.Marshal(noteContent)
	if err != nil {
		return nil, err
	}
	id := rawJSONText(item.ID)
	if id == "" {
		id = fmt.Sprintf("%d", fallbackPublicNoteID(code, contentBytes, "id"))
	}
	contentID := rawJSONText(item.ContentID)
	if contentID == "" {
		contentID = fmt.Sprintf("%d", fallbackPublicNoteID(code, contentBytes, "content"))
	}
	msg := map[string]any{
		"id":             id,
		"contentId":      contentID,
		"content":        string(contentBytes),
		"type":           "MESSAGE_TYPE_NOTES",
		"pairId":         publicNotePairID,
		"receiverRoomId": "0",
		"roomName":       firstNonEmpty(item.RoomName, "公开笔记"),
		"sender":         item.Sender,
		"senderDno":      item.SenderDno,
		"senderUser":     rawJSONValue(item.SenderUser, map[string]any{}),
		"upId":           fmt.Sprint(upID),
		"createTime":     fmt.Sprint(createTime),
	}
	if msg["upId"] == "" {
		msg["upId"] = id
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func normalizePublicNoteItems(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			data, err := json.Marshal(item)
			if err != nil {
				continue
			}
			if err = json.Unmarshal(data, &raw); err != nil {
				continue
			}
		}
		normalized := make(map[string]any, len(raw))
		for k, v := range raw {
			normalized[k] = v
		}
		normalized["type"] = publicNoteString(raw["type"])
		normalized["title"] = publicNoteString(raw["title"])
		normalized["subTitle"] = publicNoteString(raw["subTitle"])
		normalized["content"] = publicNoteString(raw["content"])
		normalized["duration"] = publicNoteInt(raw["duration"])
		normalized["verifyVideo"] = publicNoteBool(raw["verifyVideo"])
		normalized["aspectRatio"] = publicNoteFloat(raw["aspectRatio"])
		normalized["tgFileId"] = publicNoteString(raw["tgFileId"])
		out = append(out, normalized)
	}
	return out
}

func publicNoteString(v any) string {
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func publicNoteInt(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		i, _ := value.Int64()
		return int(i)
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return 0
		}
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return int(i)
		}
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return int(f)
		}
	}
	return 0
}

func publicNoteFloat(v any) float64 {
	switch value := v.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case json.Number:
		f, _ := value.Float64()
		return f
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return 0
		}
		f, _ := strconv.ParseFloat(value, 64)
		return f
	}
	return 0
}

func publicNoteBool(v any) bool {
	switch value := v.(type) {
	case bool:
		return value
	case string:
		value = strings.TrimSpace(strings.ToLower(value))
		return value == "1" || value == "true" || value == "yes"
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	case json.Number:
		i, _ := value.Int64()
		return i != 0
	}
	return false
}

func rawJSONInt64(raw json.RawMessage, fallback int64) int64 {
	text := rawJSONText(raw)
	if text == "" || text == "null" {
		return fallback
	}
	if v, err := strconv.ParseInt(text, 10, 64); err == nil {
		return v
	}
	if f, err := strconv.ParseFloat(text, 64); err == nil {
		return int64(f)
	}
	return fallback
}

func rawJSONValue(raw json.RawMessage, fallback any) any {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || string(raw) == "null" {
		return fallback
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return fallback
	}
	return out
}

func rawJSONText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String()
	}
	return strings.Trim(strings.TrimSpace(string(raw)), `"`)
}

func fallbackPublicNoteID(code string, value any, salt string) int64 {
	data, _ := json.Marshal(value)
	sum := sha1.Sum([]byte(code + ":" + salt + ":" + string(data)))
	hexText := hex.EncodeToString(sum[:8])
	v, _ := strconv.ParseInt(hexText[:15], 16, 64)
	if v < 0 {
		v = -v
	}
	return v
}
