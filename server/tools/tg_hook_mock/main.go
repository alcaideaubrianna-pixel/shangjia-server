// tg_hook_mock is a deterministic Telegram Bot API replacement for local
// integration tests. It never contacts Telegram.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rule struct {
	Method         string `json:"method"`
	Action         string `json:"action"` // success, error, timeout
	DelayMs        int    `json:"delayMs"`
	WebhookDelayMs int    `json:"webhookDelayMs"`
	Code           int    `json:"code"`
	Message        string `json:"message"`
	ChatID         string `json:"chatId"`
}

type scenario struct {
	Default rule   `json:"default"`
	Rules   []rule `json:"rules"`
}

type recorder struct {
	sync.Mutex
	Calls []map[string]any `json:"calls"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:19100", "listen address")
	scenarioPath := flag.String("scenario", "", "scenario JSON")
	recordPath := flag.String("record", "", "write call record JSON on shutdown")
	recordFormat := flag.String("record-format", "json", "call record format: json or jsonl")
	webhookURL := flag.String("webhook-url", "", "post synthetic Telegram updates to this webhook URL")
	webhookKey := flag.String("webhook-key", "", "webhook key query parameter")
	flag.Parse()
	if *recordFormat != "json" && *recordFormat != "jsonl" {
		panic("record-format must be json or jsonl")
	}
	var recordEncoder *json.Encoder
	if *recordPath != "" && *recordFormat == "jsonl" {
		file, err := os.OpenFile(*recordPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			panic(err)
		}
		defer file.Close()
		recordEncoder = json.NewEncoder(file)
	}

	var sc scenario
	if *scenarioPath != "" {
		data, err := os.ReadFile(*scenarioPath)
		if err != nil {
			panic(err)
		}
		if err = json.Unmarshal(data, &sc); err != nil {
			panic(err)
		}
	}
	if sc.Default.Action == "" {
		sc.Default.Action = "success"
	}
	var calls recorder
	var scenarioMu sync.RWMutex
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 本地故障注入不重启Hook，避免切换场景时打断其他频道的请求。
		if r.URL.Path == "/__test/scenario" {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var next scenario
			if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&next); err != nil {
				http.Error(w, "invalid scenario", http.StatusBadRequest)
				return
			}
			if next.Default.Action == "" {
				next.Default.Action = "success"
			}
			scenarioMu.Lock()
			sc = next
			scenarioMu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		// Telegram Bot API 路径包含任意 token，不能只匹配测试 token。
		method := strings.TrimPrefix(r.URL.Path, "/")
		if slash := strings.IndexByte(method, '/'); slash >= 0 {
			method = method[slash+1:]
		}
		method = strings.TrimSuffix(method, "/")
		payload, err := decodeTelegramPayload(r)
		if err != nil {
			http.Error(w, "invalid Telegram payload", http.StatusBadRequest)
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		requestChatID := fmt.Sprint(payload["chat_id"])
		if requestChatID == "<nil>" || requestChatID == "" {
			requestChatID = r.URL.Query().Get("chat_id")
		}
		calls.Lock()
		media, _ := payload["media"].([]any)
		call := map[string]any{"method": method, "at": time.Now().UTC(), "chat_id": requestChatID, "media_count": len(media)}
		if r.MultipartForm != nil {
			call["upload_files"] = len(r.MultipartForm.File)
		}
		for key, count := range telegramMediaInputCounts(method, payload) {
			call[key] = count
		}
		if recordEncoder != nil {
			// 压测按行追加，不在每个请求上重写全部历史或无限保留内存记录。
			_ = recordEncoder.Encode(call)
		} else {
			calls.Calls = append(calls.Calls, call)
		}
		// 每次请求立即落盘，进程被测试脚本终止时仍能保留调用证据。
		if *recordPath != "" && recordEncoder == nil {
			if data, err := json.Marshal(calls); err == nil {
				_ = os.WriteFile(*recordPath, data, 0o600)
			}
		}
		calls.Unlock()
		scenarioMu.RLock()
		selected := sc.Default
		for _, candidate := range sc.Rules {
			if candidate.Method == method && (candidate.ChatID == "" || candidate.ChatID == requestChatID) {
				selected = candidate
				break
			}
		}
		scenarioMu.RUnlock()
		if selected.DelayMs > 0 {
			time.Sleep(time.Duration(selected.DelayMs) * time.Millisecond)
		}
		if selected.Action == "timeout" {
			return
		}
		if selected.Action == "error" {
			if selected.Code == 0 {
				selected.Code = 500
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(selected.Code)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error_code": selected.Code, "description": selected.Message})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		result := resultFor(method, requestChatID, len(media))
		if method == "sendMediaGroup" {
			for index, message := range result.([]map[string]any) {
				if index < len(media) {
					if item, ok := media[index].(map[string]any); ok {
						addMockMessageMedia(message, fmt.Sprint(item["type"]))
					}
				}
			}
		}
		attachTelegramResultPayload(method, result, payload)
		if selected.Action != "accepted_timeout" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
		}
		if *webhookURL != "" && (method == "sendMessage" || method == "sendPhoto" || method == "sendVideo" || method == "sendMediaGroup") {
			go func() {
				if selected.WebhookDelayMs > 0 {
					time.Sleep(time.Duration(selected.WebhookDelayMs) * time.Millisecond)
				}
				postSyntheticWebhook(*webhookURL, *webhookKey, result)
			}()
		}
	})
	server := &http.Server{Addr: *addr, Handler: h}
	fmt.Printf("TG_HOOK_MOCK_URL=http://%s\n", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
	if *recordPath != "" && recordEncoder == nil {
		data, _ := json.MarshalIndent(calls, "", "  ")
		_ = os.WriteFile(*recordPath, data, 0o600)
	}
}

// postSyntheticWebhook 模拟 Telegram 在发送成功后回调 Gateway Webhook。
// postSyntheticWebhook 回传与API结果完全一致的消息ID、媒体及内部对账标记，模拟已接受但响应丢失的发送。
func postSyntheticWebhook(endpoint, key string, result any) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return
	}
	q := u.Query()
	if key != "" {
		q.Set("key", key)
	}
	u.RawQuery = q.Encode()
	var messages []map[string]any
	switch value := result.(type) {
	case map[string]any:
		messages = []map[string]any{value}
	case []map[string]any:
		messages = value
	default:
		return
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, message := range messages {
		body, err := json.Marshal(map[string]any{"update_id": time.Now().UnixNano(), "channel_post": message})
		if err != nil {
			return
		}
		req, err := http.NewRequest(http.MethodPost, u.String(), strings.NewReader(string(body)))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
			_ = response.Body.Close()
		}
	}
}

func resultFor(method, chatID string, mediaCounts ...int) any {
	if chatID == "" {
		chatID = "-1000000000000"
	}
	switch method {
	case "getMe":
		return map[string]any{"id": 1, "is_bot": true, "first_name": "local-test", "username": "local_test_bot"}
	case "sendMessage", "sendPhoto", "sendVideo":
		message := map[string]any{"message_id": time.Now().UnixNano() % 1000000000, "chat": map[string]any{"id": telegramChatIDValue(chatID)}}
		if method == "sendPhoto" {
			addMockMessageMedia(message, "photo")
		}
		if method == "sendVideo" {
			addMockMessageMedia(message, "video")
		}
		return message
	case "sendMediaGroup":
		count := 1
		if len(mediaCounts) > 0 && mediaCounts[0] > 0 {
			count = mediaCounts[0]
		}
		messages := make([]map[string]any, 0, count)
		firstID := time.Now().UnixNano() % 1000000000
		for i := 0; i < count; i++ {
			message := map[string]any{"message_id": firstID + int64(i), "chat": map[string]any{"id": telegramChatIDValue(chatID)}}
			addMockMessageMedia(message, "photo")
			messages = append(messages, message)
		}
		return messages
	case "getWebhookInfo":
		return map[string]any{"url": "", "pending_update_count": 0, "allowed_updates": []string{"message", "callback_query", "my_chat_member", "chat_member"}}
	default:
		return true
	}
}

// addMockMessageMedia 返回真实 Bot API 所需的媒体类型字段，供业务按响应匹配媒体而非盲目按位置确认。
func addMockMessageMedia(message map[string]any, mediaType string) {
	file := map[string]any{"file_id": fmt.Sprintf("local-file-%v", message["message_id"]), "file_unique_id": fmt.Sprintf("local-unique-%v", message["message_id"]), "width": 32, "height": 32, "file_size": 516}
	if mediaType == "video" {
		delete(message, "photo")
		file["duration"] = 1
		message["video"] = file
	} else {
		delete(message, "video")
		message["photo"] = []map[string]any{file}
	}
}

// decodeTelegramPayload 同时支持 Telegram SDK 的 JSON、表单和 multipart 上传，不重复读取请求体。
func decodeTelegramPayload(r *http.Request) (map[string]any, error) {
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType == "multipart/form-data" || contentType == "application/x-www-form-urlencoded" {
		var err error
		if contentType == "multipart/form-data" {
			err = r.ParseMultipartForm(32 << 20)
		} else {
			err = r.ParseForm()
		}
		if err != nil {
			return nil, err
		}
		payload := map[string]any{}
		for key, values := range r.PostForm {
			if len(values) > 0 {
				payload[key] = values[0]
			}
		}
		if raw, ok := payload["media"].(string); ok {
			var media []any
			if err := json.Unmarshal([]byte(raw), &media); err != nil {
				return nil, err
			}
			payload["media"] = media
		}
		return payload, nil
	}
	payload := map[string]any{}
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if r.ContentLength == 0 {
		return payload, nil
	}
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func telegramChatIDValue(value string) int64 {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return -1000000000000
	}
	return id
}
