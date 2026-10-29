package sys

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestTelegramDeleteMessagesBatchRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bot123:token/deleteMessages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("chat_id") != "-1001" || r.Form.Get("message_ids") != "[1,2,3,4,5,6,7,8,9,10]" {
			t.Fatalf("unexpected form: %v", r.Form)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer server.Close()
	t.Setenv("YOUBAN_TELEGRAM_RUNTIME_API_URL", server.URL)
	oldFallback := os.Getenv("YOUBAN_TELEGRAM_BOT_API_URL")
	defer os.Setenv("YOUBAN_TELEGRAM_BOT_API_URL", oldFallback)
	messages := make([]telegramDeleteMessage, 10)
	for i := range messages {
		messages[i] = telegramDeleteMessage{Id: int64(i + 1), TargetChatId: "-1001", MessageId: int64(i + 1)}
	}
	if err := telegramDeleteMessagesBatch(context.Background(), "123:token", messages); err != nil {
		t.Fatalf("batch delete request failed: %v", err)
	}
}
