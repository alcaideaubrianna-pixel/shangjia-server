package sys

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestPrepareTelegramPhotoUploadFileKeepsOutputWithinLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 2400, 2400))
	for y := 0; y < 2400; y++ {
		for x := 0; x < 2400; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	if err = png.Encode(file, img); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	outPath, cleanup, err := prepareTelegramPhotoUploadFile(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	size, err := fileSize(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if size > telegramPhotoMaxUploadBytes {
		t.Fatalf("output size=%d exceeds limit=%d", size, telegramPhotoMaxUploadBytes)
	}
}

func TestSplitTelegramMediaItems(t *testing.T) {
	media := make([]*telegramMediaItem, 23)
	chunks := splitTelegramMediaItems(media, telegramMediaGroupMaxItems)
	if len(chunks) != 3 {
		t.Fatalf("chunks=%d, want 3", len(chunks))
	}
	if len(chunks[0]) != 10 || len(chunks[1]) != 10 || len(chunks[2]) != 3 {
		t.Fatalf("chunk sizes=%d,%d,%d, want 10,10,3", len(chunks[0]), len(chunks[1]), len(chunks[2]))
	}
}

func TestTelegramSingleVideoUsesFileIdWithCover(t *testing.T) {
	media := &telegramMediaItem{MediaType: "video", TgFileId: "video-file-id", AntiScanEnabled: true}
	if !telegramVideoUsesReusableFileIdWithCover(media) {
		t.Fatal("anti-scan single video should reuse its file_id with a new cover")
	}
	input, closer, err := telegramSingleMediaInputFile(context.Background(), media)
	if err != nil {
		t.Fatal(err)
	}
	if closer != nil {
		t.Fatal("file_id input should not allocate a closer")
	}
	value, ok := input.(*models.InputFileString)
	if !ok || value.Data != media.TgFileId {
		t.Fatalf("unexpected video input: %#v", input)
	}
}

func TestPrepareTelegramMediaKeepsVideoFileIdForSingleCover(t *testing.T) {
	media := &telegramMediaItem{MediaType: "video", TgFileId: "video-file-id", TgThumbFileId: "old-thumb", AntiScanEnabled: true}
	(&sSysPublish{}).prepareTelegramMediaItemForSend(context.Background(), media)
	if media.TgFileId != "video-file-id" {
		t.Fatalf("video file_id was cleared: %q", media.TgFileId)
	}
	if media.TgThumbFileId != "" {
		t.Fatalf("old thumbnail file_id was retained: %q", media.TgThumbFileId)
	}
}

func TestCachedTelegramVideoPosterFilePrefersStoredPoster(t *testing.T) {
	posterPath := filepath.Join(t.TempDir(), "poster.jpg")
	if err := os.WriteFile(posterPath, []byte("poster"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, cleanup, err := cachedTelegramVideoPosterFile(context.Background(), &telegramMediaItem{
		MediaType:         "video",
		PosterStoragePath: posterPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		t.Fatal("stored poster should not require cleanup")
	}
	if path != posterPath {
		t.Fatalf("poster path=%q want=%q", path, posterPath)
	}
}

func TestNormalizeTelegramPhotoDimensions(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "large square", width: 8000, height: 8000},
		{name: "wide banner", width: 12000, height: 300},
		{name: "tall banner", width: 300, height: 12000},
		{name: "valid photo", width: 1920, height: 1080},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, tt.width, tt.height))
			out := normalizeTelegramPhotoDimensions(img)
			if out == nil {
				t.Fatal("expected image")
			}
			bounds := out.Bounds()
			if !telegramPhotoDimensionsValid(bounds.Dx(), bounds.Dy()) {
				t.Fatalf("invalid dimensions after normalization: %dx%d", bounds.Dx(), bounds.Dy())
			}
		})
	}
}
