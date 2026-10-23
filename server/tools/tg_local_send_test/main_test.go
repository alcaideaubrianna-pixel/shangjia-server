package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunUsesCustomEndpointAndCleansReturnedMessages(t *testing.T) {
	t.Setenv(tokenEnv, "123456:test-token")
	var mu sync.Mutex
	calls := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := filepath.Base(r.URL.Path)
		mu.Lock()
		calls[method]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "getMe":
			fmt.Fprint(w, `{"ok":true,"result":{"id":123456,"is_bot":true,"first_name":"Test","username":"local_test_bot"}}`)
		case "sendMediaGroup":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse album multipart: %v", err)
			}
			fmt.Fprint(w, `{"ok":true,"result":[{"message_id":101,"date":1,"chat":{"id":-1001,"type":"channel"}},{"message_id":102,"date":1,"chat":{"id":-1001,"type":"channel"}}]}`)
		case "sendVideo":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse video multipart: %v", err)
			}
			fmt.Fprint(w, `{"ok":true,"result":{"message_id":103,"date":1,"chat":{"id":-1001,"type":"channel"},"video":{"file_id":"video","file_unique_id":"unique","width":1,"height":1,"duration":1}}}`)
		case "deleteMessage":
			fmt.Fprint(w, `{"ok":true,"result":true}`)
		default:
			http.Error(w, "unexpected method", http.StatusNotFound)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	mediaPaths := make([]string, 3)
	for index := range mediaPaths {
		ext := ".jpg"
		if index == 2 {
			ext = ".mp4"
		}
		mediaPaths[index] = filepath.Join(dir, fmt.Sprintf("media-%d%s", index, ext))
		if index == 2 {
			if err := os.WriteFile(mediaPaths[index], []byte(strings.Repeat("x", index+1)), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		file, err := os.Create(mediaPaths[index])
		if err != nil {
			t.Fatal(err)
		}
		img := image.NewRGBA(image.Rect(0, 0, 2, 2))
		img.Set(0, 0, color.White)
		if err = jpeg.Encode(file, img, nil); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	manifestJSON := fmt.Sprintf(`{"display":[{"source":%q,"filename":"1.jpg"},{"source":%q,"filename":"2.jpg"}],"verify":{"source":%q,"filename":"verify.mp4"}}`, mediaPaths[0], mediaPaths[1], mediaPaths[2])
	if err := os.WriteFile(manifestPath, []byte(manifestJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	err := run(context.Background(), options{
		endpoint: server.URL,
		chatID:   "@test_channel",
		manifest: manifestPath,
		repeat:   1,
		timeout:  time.Minute,
		cleanup:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["getMe"] != 1 || calls["sendMediaGroup"] != 1 || calls["sendVideo"] != 1 || calls["deleteMessage"] != 3 {
		t.Fatalf("unexpected API calls: %#v", calls)
	}
}

func TestReadManifestRejectsInvalidAlbumSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"display":[{"source":"one.jpg"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifest(path); err == nil || !strings.Contains(err.Error(), "2 to 10") {
		t.Fatalf("expected album size error, got %v", err)
	}
}

func TestValidateOptionsRejectsNonHTTPEndpoint(t *testing.T) {
	err := validateOptions(options{endpoint: "file:///tmp/bot-api", chatID: "@test", manifest: "manifest.json", repeat: 1, timeout: time.Minute})
	if err == nil {
		t.Fatal("expected invalid endpoint error")
	}
}
