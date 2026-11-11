package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/go-telegram/bot/models"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestDecodeTelegramPayloadFormats 验证真实 SDK 使用的三种请求格式不会丢失频道和相册信息。
func TestDecodeTelegramPayloadFormats(t *testing.T) {
	const chatID = "-100999000095"
	for _, format := range []string{"json", "form", "multipart"} {
		t.Run(format, func(t *testing.T) {
			var request *http.Request
			switch format {
			case "json":
				request = httptest.NewRequest("POST", "/sendMediaGroup", strings.NewReader(fmt.Sprintf(`{"chat_id":%s,"media":[{"type":"photo","media":"file1"},{"type":"photo","media":"file2"}]}`, chatID)))
				request.Header.Set("Content-Type", "application/json")
			case "form":
				request = httptest.NewRequest("POST", "/sendMediaGroup", strings.NewReader("chat_id="+chatID+"&media=%5B%7B%22type%22%3A%22photo%22%7D%2C%7B%22type%22%3A%22photo%22%7D%5D"))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			case "multipart":
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				if err := writer.WriteField("chat_id", chatID); err != nil {
					t.Fatal(err)
				}
				if err := writer.WriteField("media", `[{"type":"photo","media":"attach://image1"},{"type":"photo","media":"attach://image2"}]`); err != nil {
					t.Fatal(err)
				}
				file, err := writer.CreateFormFile("image1", "image.jpg")
				if err != nil {
					t.Fatal(err)
				}
				if _, err = file.Write([]byte("local-image")); err != nil {
					t.Fatal(err)
				}
				if err = writer.Close(); err != nil {
					t.Fatal(err)
				}
				request = httptest.NewRequest("POST", "/sendMediaGroup", &body)
				request.Header.Set("Content-Type", writer.FormDataContentType())
			}
			payload, err := decodeTelegramPayload(request)
			if err != nil {
				t.Fatal(err)
			}
			if request.MultipartForm != nil {
				defer request.MultipartForm.RemoveAll()
			}
			if fmt.Sprint(payload["chat_id"]) != chatID {
				t.Fatalf("chat_id=%v", payload["chat_id"])
			}
			media, ok := payload["media"].([]any)
			if !ok || len(media) != 2 {
				t.Fatalf("media=%v", payload["media"])
			}
			messages := resultFor("sendMediaGroup", fmt.Sprint(payload["chat_id"]), len(media)).([]map[string]any)
			if len(messages) != 2 {
				t.Fatalf("messages=%d", len(messages))
			}
			for _, message := range messages {
				if message["chat"].(map[string]any)["id"] != int64(-100999000095) {
					t.Fatalf("wrong chat: %v", message)
				}
			}
			encoded, err := json.Marshal(messages)
			if err != nil {
				t.Fatal(err)
			}
			var decoded []models.Message
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if len(decoded) != 2 || len(decoded[0].Photo) == 0 || len(decoded[1].Photo) == 0 {
				t.Fatal("SDK media matching requires photo fields on each response message")
			}
			if messages[0]["message_id"] == messages[1]["message_id"] {
				t.Fatal("duplicate message IDs")
			}
		})
	}
}

// TestWebhookPreservesDeliveryEvidence 验证响应和回调使用同一组消息，隐形标记与媒体类型不会在Mock层丢失。
func TestWebhookPreservesDeliveryEvidence(t *testing.T) {
	const marker = "caption\u2063attempt-token\u2064"
	payload := map[string]any{"media": []any{map[string]any{"type": "photo", "media": "cached-file-id", "caption": marker}, map[string]any{"type": "video", "media": "attach://video"}}}
	result := resultFor("sendMediaGroup", "-100999000095", 2)
	attachTelegramResultPayload("sendMediaGroup", result, payload)
	messages := result.([]map[string]any)
	addMockMessageMedia(messages[1], "video")
	updates := make(chan models.Update, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "local-key" {
			t.Error("webhook key missing")
		}
		var update models.Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			t.Error(err)
			return
		}
		updates <- update
	}))
	defer server.Close()
	postSyntheticWebhook(server.URL, "local-key", result)
	for index := 0; index < 2; index++ {
		select {
		case update := <-updates:
			if update.ChannelPost == nil || int64(update.ChannelPost.ID) != messages[index]["message_id"].(int64) {
				t.Fatal("webhook changed message ID", update)
			}
			if index == 0 && update.ChannelPost.Caption != marker {
				t.Fatal("webhook lost delivery marker")
			}
		case <-time.After(time.Second):
			t.Fatal("missing webhook")
		}
	}
	counts := telegramMediaInputCounts("sendMediaGroup", payload)
	if counts["file_id_count"] != 1 || counts["upload_count"] != 1 {
		t.Fatal(counts)
	}
}
