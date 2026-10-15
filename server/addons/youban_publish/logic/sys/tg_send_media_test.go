package sys

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTelegramMediaRemoteURLSelection(t *testing.T) {
	media := &telegramMediaItem{FileUrl: "https://cdn.example/media/video.mp4"}
	if got := telegramMediaRemoteURL(media); got != media.FileUrl {
		t.Fatalf("remote URL = %q, want %q", got, media.FileUrl)
	}
	media.ForceUpload = true
	if got := telegramMediaRemoteURL(media); got != "" {
		t.Fatalf("forced upload must reject URL, got %q", got)
	}
	media.ForceUpload = false
	media.AntiScanEnabled = true
	if got := telegramMediaRemoteURL(media); got != "" {
		t.Fatalf("sanitized media must reject original URL, got %q", got)
	}
}

func TestApplySameBotCollectFileIDs(t *testing.T) {
	media := []*telegramMediaItem{
		{MediaType: "image", CollectedFileIdEligible: true},
		{MediaType: "image", CollectedFileIdEligible: false},
		{MediaType: "video", CollectedFileIdEligible: true},
	}
	sources := []sameBotCollectFile{
		{MediaType: "photo", SourceFileId: "AgAC-first"},
		{MediaType: "photo", SourceFileId: "AgAC-edited"},
		{MediaType: "video", SourceFileId: "BAAC-video"},
	}
	applySameBotCollectFileIDs(media, sources)
	if media[0].TgFileId != "AgAC-first" {
		t.Fatalf("same Bot photo file_id not reused: %+v", media[0])
	}
	if media[1].TgFileId != "" {
		t.Fatalf("edited media must not reuse collected file_id: %+v", media[1])
	}
	if media[2].TgFileId != "BAAC-video" {
		t.Fatalf("same Bot video file_id not reused: %+v", media[2])
	}
	media[0].AntiScanEnabled = true
	if telegramMediaUsesReusableFileId(media[0]) {
		t.Fatal("anti-scan media must upload processed bytes instead of reusing collected file_id")
	}
}

func TestTelegramRemoteURLFallbackIsDeterministic(t *testing.T) {
	media := &telegramMediaItem{FileUrl: "https://cdn.example/media/video.mp4"}
	for _, message := range []string{
		"Bad Request: failed to get HTTP URL content",
		`Bad Request: failed to send message #1 with the error message "WEBPAGE_CURL_FAILED"`,
		`Bad Request: failed to send message #1 with the error message "WEBPAGE_MEDIA_EMPTY"`,
	} {
		if !telegramMediaCanFallbackToUpload(media, errors.New(message)) {
			t.Fatalf("explicit Telegram URL rejection should fall back to multipart: %s", message)
		}
	}
	for _, err := range []error{context.DeadlineExceeded, errors.New("unexpected EOF"), errors.New("connection reset by peer")} {
		if telegramMediaCanFallbackToUpload(media, err) {
			t.Fatalf("ambiguous delivery must not retry upload: %v", err)
		}
	}
}

func TestTelegramReusableFileFallbackTriesURLBeforeUpload(t *testing.T) {
	media := &telegramMediaItem{TgFileId: "BAAC-cached", FileUrl: "https://cdn.example/media/video.mp4"}
	fallback := telegramMediaFallbackCandidate(media)
	if fallback.TgFileId != "" || fallback.ForceUpload || telegramMediaRemoteURL(fallback) == "" {
		t.Fatalf("file_id fallback should preserve URL stage: %+v", fallback)
	}
	fallback = telegramMediaFallbackCandidate(fallback)
	if !fallback.ForceUpload {
		t.Fatalf("URL fallback should force multipart upload: %+v", fallback)
	}
}

func TestTelegramMediaGroupFallbackTriesURLBeforeUpload(t *testing.T) {
	media := []*telegramMediaItem{{TgFileId: "BAAC-cached", FileUrl: "https://cdn.example/video.mp4"}}
	urlStage := telegramMediaSetWithoutReusableFileID(media)
	if len(urlStage) != 1 || urlStage[0].TgFileId != "" || urlStage[0].ForceUpload || telegramMediaRemoteURL(urlStage[0]) == "" {
		t.Fatalf("media group URL stage is invalid: %+v", urlStage)
	}
	uploadStage := telegramMediaSetForcedUpload(urlStage)
	if len(uploadStage) != 1 || !uploadStage[0].ForceUpload || telegramMediaRemoteURL(uploadStage[0]) != "" {
		t.Fatalf("media group upload stage is invalid: %+v", uploadStage)
	}
}

func TestValidateTelegramMediaPurpose(t *testing.T) {
	tests := []struct {
		name    string
		purpose string
		media   []*telegramMediaItem
		wantErr bool
	}{
		{name: "display only", purpose: "display", media: []*telegramMediaItem{{Purpose: "display"}}},
		{name: "verify only", purpose: "verify", media: []*telegramMediaItem{{Purpose: "verify"}}},
		{name: "mixed purposes", purpose: "display", media: []*telegramMediaItem{{Purpose: "display"}, {Purpose: "verify"}}, wantErr: true},
		{name: "wrong purpose", purpose: "verify", media: []*telegramMediaItem{{Purpose: "display"}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTelegramMediaPurpose(test.purpose, test.media)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateTelegramMediaPurpose() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestTelegramMediaSetHasCompleteCopyRefs(t *testing.T) {
	complete := []*telegramMediaItem{
		{TgFileId: "copy:123:10"},
		{TgFileId: "copy:123:11"},
	}
	mixed := []*telegramMediaItem{
		{TgFileId: "copy:123:10"},
		{TgFileId: "BAACAgEAA-test"},
	}

	if !telegramMediaSetHasCompleteCopyRefs(complete) {
		t.Fatal("complete copy references should use Telegram group copy")
	}
	if telegramMediaSetHasCompleteCopyRefs(mixed) {
		t.Fatal("mixed copy references must fall back to persistent media upload")
	}
}

func TestTelegramMediaSetWithoutCopyRefs(t *testing.T) {
	media := []*telegramMediaItem{
		{TgFileId: "copy:123:10", TgThumbFileId: "thumb-copy"},
		{TgFileId: "BAACAgEAA-video", TgThumbFileId: "thumb-video"},
	}

	result := telegramMediaSetWithoutCopyRefs(media)
	if result[0].TgFileId != "" || result[0].TgThumbFileId != "" || !result[0].ForceUpload {
		t.Fatalf("copy reference was not converted to upload: %+v", result[0])
	}
	if result[1].TgFileId != media[1].TgFileId || result[1].TgThumbFileId != media[1].TgThumbFileId || result[1].ForceUpload {
		t.Fatalf("reusable file_id should be preserved: %+v", result[1])
	}
	if media[0].TgFileId == "" || media[0].ForceUpload {
		t.Fatal("source media must not be mutated")
	}
}

func TestTelegramVideoPreviewLocalPoster(t *testing.T) {
	path := filepath.Join(t.TempDir(), "original.jpg")
	if err := os.WriteFile(path, []byte("original-telegram-poster"), 0600); err != nil {
		t.Fatal(err)
	}
	media := &telegramMediaItem{MediaType: "video", PosterStoragePath: path, FileUrl: "invalid-video-source"}
	file, closer, err := telegramVideoPreview(context.Background(), media, "thumbnail")
	if err != nil || file == nil || closer == nil {
		t.Fatalf("original poster not reused: %v", err)
	}
	_ = closer.Close()
}

func TestTelegramVideoPreviewFailureIsOptional(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	file, closer, err := telegramVideoPreview(ctx, &telegramMediaItem{MediaType: "video", FileUrl: "https://%invalid"}, "thumbnail")
	if err != nil || file != nil || closer != nil {
		t.Fatalf("missing preview must not stop video: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatal("preview canceled sending context")
	}
}

func TestTelegramVideoPreviewParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := telegramVideoPreview(ctx, &telegramMediaItem{MediaType: "video"}, "thumbnail")
	if err == nil {
		t.Fatal("parent cancellation must not be swallowed")
	}
}
