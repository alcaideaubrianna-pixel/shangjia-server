package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgbot "github.com/go-telegram/bot"
)

func TestRelayPackageAlbumMarkersAndSingleTail(t *testing.T) {
	var calls, received int
	token := telegramAttemptMarker("relay-package:25:1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "sendMediaGroup") {
			var media []struct{ Type, Media, Caption string }
			if err := json.Unmarshal([]byte(r.FormValue("media")), &media); err != nil {
				t.Error(err)
				return
			}
			if len(media) != 10 {
				t.Errorf("album size=%d", len(media))
			}
			results := []map[string]any{}
			for _, m := range media {
				marker := relayPackageMarker(5, received)
				if !strings.Contains(m.Caption, marker) || !strings.Contains(m.Caption, token) || m.Media != fmt.Sprintf("cached-%d", received) {
					t.Errorf("lost marker/file ID at %d: %+v", received, m)
				}
				received++
				results = append(results, map[string]any{"message_id": received, "date": 1, "chat": map[string]any{"id": -1001, "type": "channel"}, "photo": []map[string]any{{"file_id": m.Media, "file_unique_id": m.Media, "width": 10, "height": 10}}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": results})
		} else if strings.HasSuffix(r.URL.Path, "sendPhoto") {
			if !strings.Contains(r.FormValue("caption"), relayPackageMarker(5, 10)) || !strings.Contains(r.FormValue("caption"), token) || r.FormValue("photo") != "cached-10" {
				t.Error("single tail lost markers or file ID")
			}
			received++
			_, _ = fmt.Fprint(w, `{"ok":true,"result":{"message_id":11,"date":1,"chat":{"id":-1001,"type":"channel"},"photo":[{"file_id":"cached-10","file_unique_id":"u10","width":10,"height":10}]}}`)
		} else {
			t.Errorf("unexpected API call: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	bot, err := tgbot.New("local-test", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	media := []*telegramMediaItem{}
	for i := 0; i < 11; i++ {
		media = append(media, &telegramMediaItem{Id: int64(i + 1), MediaType: "photo", Purpose: "display", TgFileId: fmt.Sprintf("cached-%d", i), DeliveryMarker: relayPackageMarker(5, i) + token})
	}
	ctx := context.WithValue(context.Background(), relayPackageContextKey{}, true)
	sent, err := new(sSysPublish).sendTelegramMediaSet(ctx, bot, "-1001", "display", "<b>资料文案</b>", media)
	if err != nil || len(sent) != 11 || calls != 2 || received != 11 {
		t.Fatalf("stage result: count=%d calls=%d received=%d err=%v", len(sent), calls, received, err)
	}
	for i, m := range sent {
		if m.MediaId != int64(i+1) || m.Purpose != "display" {
			t.Fatalf("lost media ownership: %+v", m)
		}
	}
}
