package sys

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tgbot "github.com/go-telegram/bot"

	"hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/internal/library/telegrammedia"
)

func TestGatewayDownloadsOfficialImagesAndLocalLargeVideo(t *testing.T) {
	var photo bytes.Buffer
	if err := jpeg.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	video := []byte("local-video-content")
	var cloudRequests, localRequests atomic.Int32
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cloudRequests.Add(1)
		if strings.HasSuffix(r.URL.Path, "/getFile") {
			fmt.Fprint(w, `{"ok":true,"result":{"file_id":"photo","file_unique_id":"unique","file_path":"photos/file.jpg"}}`)
			return
		}
		if !strings.Contains(r.URL.Path, "/file/bot") {
			t.Errorf("unexpected official path: %s", r.URL.Path)
		}
		_, _ = w.Write(photo.Bytes())
	}))
	defer cloud.Close()
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		localRequests.Add(1)
		if strings.HasSuffix(r.URL.Path, "/getFile") {
			fmt.Fprint(w, `{"ok":true,"result":{"file_id":"video","file_path":"/var/lib/telegram-bot-api/123/video.mp4"}}`)
			return
		}
		if r.URL.Path != "/123/video.mp4" {
			t.Errorf("unexpected local file path: %s", r.URL.Path)
		}
		_, _ = w.Write(video)
	}))
	defer local.Close()
	gateway := mediaRoutingTestGateway(&service.RuntimeConfig{RuntimeServerURL: local.URL, MediaServerURL: local.URL, MediaFileURL: local.URL})
	gateway.newMediaClientForToken = func(token, proxy, endpoint string, handler tgbot.HandlerFunc) (*tgbot.Bot, error) {
		if endpoint == "" {
			endpoint = cloud.URL
		}
		return newBotWithTimeout(token, proxy, endpoint, handler, telegramMediaRequestTimeout)
	}
	// Five display photos followed by a large verification video and its preview.
	// Exercise the real gateway, SDK getFile, file server and shared byte transfer.
	for i := 0; i < 7; i++ {
		in := telegrammedia.Request{FileID: "photo", MediaType: "image", Size: 1000}
		expected := photo.Bytes()
		endpoint := "official"
		if i == 5 {
			in = telegrammedia.Request{FileID: "video", MediaType: "video", Size: 30 * 1024 * 1024}
			expected = video
			endpoint = "local"
		}
		data, result, err := gateway.DownloadMedia(context.Background(), "123456:routing-test", in, 50*1024*1024)
		if err != nil || !bytes.Equal(data, expected) || result.Endpoint != endpoint {
			t.Fatalf("media %d: endpoint=%s err=%v", i, result.Endpoint, err)
		}
		if endpoint == "official" {
			if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
				t.Fatal("photo bytes corrupted")
			}
		}
	}
	if cloudRequests.Load() != 12 || localRequests.Load() != 2 {
		t.Fatalf("incorrect API/file routes cloud=%d local=%d", cloudRequests.Load(), localRequests.Load())
	}
}

func TestGatewayLocalCircuitDoesNotBlockOfficialPhotos(t *testing.T) {
	key := tokenKey("123456:routing-test")
	mediaDownloads.Lock()
	delete(mediaDownloads.circuits, key)
	mediaDownloads.Unlock()
	defer func() { mediaDownloads.Lock(); delete(mediaDownloads.circuits, key); mediaDownloads.Unlock() }()
	var localCalls atomic.Int32
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true,"result":{"file_id":"photo","file_path":"photos/a.jpg"}}`)
	}))
	defer cloud.Close()
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		localCalls.Add(1)
		w.WriteHeader(504)
		fmt.Fprint(w, `{"ok":false,"error_code":504,"description":"gateway timeout"}`)
	}))
	defer local.Close()
	gateway := mediaRoutingTestGateway(&service.RuntimeConfig{MediaServerURL: local.URL, MediaFileURL: local.URL})
	gateway.newMediaClientForToken = func(token, proxy, endpoint string, handler tgbot.HandlerFunc) (*tgbot.Bot, error) {
		if endpoint == "" {
			endpoint = cloud.URL
		}
		return newBotWithTimeout(token, proxy, endpoint, handler, telegramMediaRequestTimeout)
	}
	for i := 0; i < 4; i++ {
		_, err := gateway.ResolveMedia(context.Background(), "123456:routing-test", telegrammedia.Request{FileID: "video", MediaType: "video", Size: 30 * 1024 * 1024})
		if err == nil {
			t.Fatal("failed local download reported success")
		}
	}
	if localCalls.Load() != 3 {
		t.Fatalf("circuit did not stop failing local calls: %d", localCalls.Load())
	}
	result, err := gateway.ResolveMedia(context.Background(), "123456:routing-test", telegrammedia.Request{FileID: "photo", MediaType: "image", Size: 1000})
	if err != nil || result.Endpoint != "official" {
		t.Fatal("local circuit blocked ordinary photo")
	}
}

func TestGatewayLargeVideoSlotsDoNotBlockOfficialPhotos(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	const key = "isolated-media-capacity-test"
	releaseOne, err := acquireMediaDownload(ctx, key, "local")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseOne()
	releaseTwo, err := acquireMediaDownload(ctx, key, "local")
	if err != nil {
		t.Fatal(err)
	}
	defer releaseTwo()
	releasePhoto, err := acquireMediaDownload(ctx, key, "official")
	if err != nil {
		t.Fatal("two large videos blocked an official photo")
	}
	releasePhoto()
}
