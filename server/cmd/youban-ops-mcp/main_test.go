package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveProfileByMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/youban_publish/ai-ops/telegram/message/context"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("chatId"), "4324969906"; got != want {
			t.Fatalf("chatId = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("messageId"), "1178"; got != want {
			t.Fatalf("messageId = %q, want %q", got, want)
		}
		if got := r.Header.Get("X-AI-Ops-Token"); got != "test-token" {
			t.Fatalf("token = %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"found":                  true,
				"profileId":              528183,
				"profileNo":              "A03214",
				"isCollected":            true,
				"collectSourceId":        351,
				"collectSourceTitle":     "百花阁 BOT 采集",
				"collectSourceUsername":  "NRGXF1",
				"collectSourceChatId":    "-1004324969906",
				"collectSourceMessageId": 1178,
				"collectSourceUrl":       "https://t.me/c/4324969906/1178",
				"collectEventId":         5018630,
				"collectEventStatus":     "done",
				"collectRuleId":          338,
				"collectRuleName":        "百花阁",
				"diagnosis":              "采集完成",
			},
		})
	}))
	defer server.Close()

	client := &apiClient{baseURL: server.URL, token: "test-token", client: server.Client()}
	_, output, err := client.resolveProfileByMessage(context.Background(), nil, messageContextInput{
		ChatID: "4324969906", MessageID: 1178,
	})
	if err != nil {
		t.Fatalf("resolveProfileByMessage() error = %v", err)
	}
	if !output.Found || !output.IsCollected {
		t.Fatalf("unexpected flags: found=%v collected=%v", output.Found, output.IsCollected)
	}
	if got, want := output.ProfileNo, "A03214"; got != want {
		t.Fatalf("profileNo = %q, want %q", got, want)
	}
	if got, want := output.CollectSourceID, int64(351); got != want {
		t.Fatalf("collectSourceId = %d, want %d", got, want)
	}
	if got, want := output.CollectSourceChatID, "-1004324969906"; got != want {
		t.Fatalf("collectSourceChatId = %q, want %q", got, want)
	}
}

func TestMessageContextUsesTelegramURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Query().Get("url"), "https://t.me/demo/12?single"; got != want {
			t.Fatalf("url = %q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"found": false}})
	}))
	defer server.Close()

	client := &apiClient{baseURL: server.URL, token: "test-token", client: server.Client()}
	if _, err := client.messageContext(context.Background(), messageContextInput{URL: "https://t.me/demo/12?single"}); err != nil {
		t.Fatalf("messageContext() error = %v", err)
	}
}

func TestMapInt64(t *testing.T) {
	data := map[string]any{
		"float":  float64(42),
		"number": json.Number("43"),
	}
	if got := mapInt64(data, "float"); got != 42 {
		t.Fatalf("float conversion = %d", got)
	}
	if got := mapInt64(data, "number"); got != 43 {
		t.Fatalf("json.Number conversion = %d", got)
	}
}
