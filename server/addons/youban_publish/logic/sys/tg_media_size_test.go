package sys

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/go-telegram/bot/models"
)

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
		{"video", 50_000_000, true},
		{"video", 50_000_001, false},
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
		if telegramMediaCanFallbackToUpload(media, err) || telegramMediaSetCanFallbackToUpload([]*telegramMediaItem{media}, err) {
			t.Fatalf("unknown delivery would be sent twice: %v", err)
		}
	}
}
