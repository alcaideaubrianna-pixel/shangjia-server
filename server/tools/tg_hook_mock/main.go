// tg_hook_mock is a deterministic Telegram Bot API replacement for local
// integration tests. It never contacts Telegram.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type rule struct {
	Method  string `json:"method"`
	Action  string `json:"action"` // success, error, timeout
	DelayMs int    `json:"delayMs"`
	Code    int    `json:"code"`
	Message string `json:"message"`
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
	flag.Parse()

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
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/botTEST/")
		method = strings.TrimSuffix(method, "/")
		calls.Lock()
		calls.Calls = append(calls.Calls, map[string]any{"method": method, "at": time.Now().UTC(), "query": r.URL.Query()})
		calls.Unlock()
		selected := sc.Default
		for _, candidate := range sc.Rules {
			if candidate.Method == method {
				selected = candidate
				break
			}
		}
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
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": resultFor(method)})
	})
	server := &http.Server{Addr: *addr, Handler: h}
	fmt.Printf("TG_HOOK_MOCK_URL=http://%s\n", *addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
	if *recordPath != "" {
		data, _ := json.MarshalIndent(calls, "", "  ")
		_ = os.WriteFile(*recordPath, data, 0o600)
	}
}

func resultFor(method string) any {
	switch method {
	case "getMe":
		return map[string]any{"id": 1, "is_bot": true, "first_name": "local-test", "username": "local_test_bot"}
	case "sendMessage", "sendPhoto", "sendVideo":
		return map[string]any{"message_id": time.Now().UnixNano() % 1000000000, "chat": map[string]any{"id": "-1000000000000"}}
	case "sendMediaGroup":
		return []map[string]any{{"message_id": time.Now().UnixNano() % 1000000000, "chat": map[string]any{"id": "-1000000000000"}}}
	case "getWebhookInfo":
		return map[string]any{"url": "", "pending_update_count": 0, "allowed_updates": []string{"message", "callback_query", "my_chat_member", "chat_member"}}
	default:
		return true
	}
}
