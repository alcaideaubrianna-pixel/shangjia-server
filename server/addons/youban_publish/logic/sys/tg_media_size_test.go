package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func TestIncidentOversizedPhotoIsCompressedBeforeSizeDecision(t *testing.T) {
	for _, originalSize := range []int64{45_859_302, 22_774_645, 16_181_637} {
		file, err := os.CreateTemp(t.TempDir(), "incident-*.png")
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(file, image.NewNRGBA(image.Rect(0, 0, 128, 128))); err != nil {
			t.Fatal(err)
		}
		// Valid image with trailing metadata reproduces the original byte
		// size without committing production media or allocating a huge image.
		if err = file.Truncate(originalSize); err != nil {
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
		media := &telegramMediaItem{MediaType: "image", Size: originalSize}
		if err = validateTelegramMediaSetKnownUploadSize([]*telegramMediaItem{media}); err != nil {
			t.Fatalf("compression bypassed for %d bytes: %v", originalSize, err)
		}
		path, cleanup, err := NewSysPublish().prepareTelegramMediaUploadFile(context.Background(), media, file.Name(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err = validateTelegramMediaUploadSize(media, path); err != nil {
			t.Fatalf("processed upload rejected: %v", err)
		}
		if cleanup != nil {
			cleanup()
		}
	}
}

func TestIncidentOversizedPhotoAlbumUsesBotAfterCompression(t *testing.T) {
	var media []*telegramMediaItem
	for i, size := range []int64{45_859_302, 22_774_645, 16_181_637} {
		file, err := os.CreateTemp(t.TempDir(), "incident-*.png")
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(file, image.NewNRGBA(image.Rect(0, 0, 128, 128))); err != nil {
			t.Fatal(err)
		}
		if err = file.Truncate(size); err != nil {
			t.Fatal(err)
		}
		if err = file.Close(); err != nil {
			t.Fatal(err)
		}
		media = append(media, &telegramMediaItem{Id: int64(i + 1), MediaType: "image", Purpose: "display", Size: size, StoragePath: file.Name()})
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/botlocal-test/sendMediaGroup" {
			t.Errorf("unexpected API: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		defer r.MultipartForm.RemoveAll()
		var inputs []struct{ Type, Media string }
		if err := json.Unmarshal([]byte(r.FormValue("media")), &inputs); err != nil || len(inputs) != 3 {
			t.Errorf("album input count=%d err=%v", len(inputs), err)
		}
		files := 0
		for _, group := range r.MultipartForm.File {
			for _, file := range group {
				files++
				if file.Size > telegramPhotoMaxUploadBytes {
					t.Errorf("uploaded original instead of compressed photo: %d", file.Size)
				}
			}
		}
		if files != 3 {
			t.Errorf("uploaded files=%d, want 3", files)
		}
		results := make([]map[string]any, 3)
		for i := range results {
			results[i] = map[string]any{"message_id": i + 1, "date": 1, "chat": map[string]any{"id": -1001, "type": "channel"}, "photo": []map[string]any{{"file_id": fmt.Sprint(i), "file_unique_id": fmt.Sprint(i), "width": 128, "height": 128}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": results})
	}))
	defer server.Close()
	bot, err := tgbot.New("local-test", tgbot.WithSkipGetMe(), tgbot.WithServerURL(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	sent, err := NewSysPublish().sendTelegramMediaSet(context.Background(), bot, "-1001", "display", "资料", media)
	if err != nil || len(sent) != 3 || calls != 1 {
		t.Fatalf("compressed album: sent=%d calls=%d err=%v", len(sent), calls, err)
	}
}

func TestTelegramMediaURLSizeLimits(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		size       int64
		wantURL    bool
	}{
		{"legacy unknown", "video", 0, true},
		{"video boundary", "video", 20_000_000, true},
		{"large video", "video", 20_000_001, false},
		{"photo boundary", "image", 5_000_000, true},
		{"large photo", "photo", 5_000_001, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			media := &telegramMediaItem{MediaType: tc.kind, Size: tc.size, FileUrl: "https://cdn.example/media.mp4"}
			if got := telegramMediaRemoteURL(media) != ""; got != tc.wantURL {
				t.Fatalf("URL selection = %t, want %t", got, tc.wantURL)
			}
		})
	}
}

func TestTelegramMediaSetOversizeHandoffBeforeSending(t *testing.T) {
	for _, tc := range []struct {
		name      string
		item      *telegramMediaItem
		wantError bool
	}{
		{"unknown", &telegramMediaItem{MediaType: "video"}, false},
		{"video boundary", &telegramMediaItem{MediaType: "video", Size: 50_000_000}, false},
		{"oversized video is compressed before upload", &telegramMediaItem{MediaType: "video", Size: 50_000_001}, false},
		{"oversized photo is compressed before upload", &telegramMediaItem{MediaType: "image", Size: 10_000_001}, false},
		{"M98925 protected photo", &telegramMediaItem{MediaType: "image", Size: 45_859_302, AntiScanEnabled: true, TgFileId: "cached"}, false},
		{"N78120 photo", &telegramMediaItem{MediaType: "image", Size: 22_774_645}, false},
		{"U73573 photo", &telegramMediaItem{MediaType: "image", Size: 16_181_637}, false},
		{"reuse", &telegramMediaItem{MediaType: "video", Size: 100_000_000, TgFileId: "cached"}, false},
		{"forced upload is compressed", &telegramMediaItem{MediaType: "video", Size: 100_000_000, TgFileId: "cached", ForceUpload: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTelegramMediaSetKnownUploadSize([]*telegramMediaItem{{MediaType: "image", Size: 100}, tc.item})
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected handoff decision: %v", err)
			}
			if err != nil && !shouldFallbackTelegramMediaToAccount(telegramPreparationError(err)) {
				t.Fatalf("oversize must use existing account fallback: %v", err)
			}
		})
	}
}

func TestTelegramMediaUploadChecksActualSize(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "upload-*")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, tc := range []struct {
		kind string
		size int64
		ok   bool
	}{
		{"video", telegramVideoMaxUploadBytes, true},
		{"video", telegramVideoMaxUploadBytes + 1, false},
		{"image", 10_000_000, true},
		{"photo", 10_000_001, false},
	} {
		if err := file.Truncate(tc.size); err != nil {
			t.Fatal(err)
		}
		media := &telegramMediaItem{MediaType: tc.kind, Size: 1}
		err := validateTelegramMediaUploadSize(media, file.Name())
		if (err == nil) != tc.ok {
			t.Fatalf("kind=%s actualSize=%d error=%v", tc.kind, tc.size, err)
		}
		if !tc.ok && (!isTelegramPermanentSendError(err) || !isTelegramMediaSizeLimitError(err)) {
			t.Fatalf("oversized media must not repeatedly block the channel: %v", err)
		}
	}
}

func TestTelegramMediaLargeVideoRetainsFileIDPriority(t *testing.T) {
	media := &telegramMediaItem{MediaType: "video", Size: 100_000_000, TgFileId: "cached-video"}
	input, closer, err := telegramSingleMediaInputFile(context.Background(), media)
	if err != nil || closer != nil {
		t.Fatalf("file_id reuse should not download or upload: %v", err)
	}
	value, ok := input.(*models.InputFileString)
	if !ok || value.Data != media.TgFileId {
		t.Fatalf("large video did not retain file_id: %#v", input)
	}
}

func TestTelegramMediaTimeoutNeverTriggersUploadFallback(t *testing.T) {
	media := &telegramMediaItem{MediaType: "video", FileUrl: "https://cdn.example/video.mp4", Size: 1}
	for _, err := range []error{context.DeadlineExceeded, context.Canceled, errors.New("unexpected EOF")} {
		if !errors.Is(err, context.DeadlineExceeded) && shouldFallbackTelegramMediaToAccount(err) {
			t.Fatalf("uncertain send must not hand off to account: %v", err)
		}
		if telegramMediaCanFallbackToUpload(media, err) || telegramMediaSetCanFallbackToUpload([]*telegramMediaItem{media}, err) {
			t.Fatalf("unknown delivery would be sent twice: %v", err)
		}
	}
}
