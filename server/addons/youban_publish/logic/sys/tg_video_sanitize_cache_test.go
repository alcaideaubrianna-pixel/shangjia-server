package sys

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCachedTelegramVideoNormalizeConvertsMOVToMP4H264AAC(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	sourcePath := t.TempDir() + "/verify.mov"
	output, err := exec.Command(ffmpegPath,
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "color=c=black:s=64x64:d=0.2",
		"-f", "lavfi", "-i", "anullsrc=r=44100:cl=mono",
		"-shortest", "-c:v", "mjpeg", "-c:a", "pcm_s16le", sourcePath,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("create MOV fixture: %v: %s", err, output)
	}
	normalizedPath, err := cachedTelegramVideoNormalize(context.Background(), ffmpegPath, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(normalizedPath)
	if !strings.EqualFold(filepath.Ext(normalizedPath), ".mp4") {
		t.Fatalf("normalized extension = %q", filepath.Ext(normalizedPath))
	}
	probe, err := exec.Command(ffprobePath,
		"-v", "error", "-show_entries", "stream=codec_type,codec_name,pix_fmt",
		"-of", "default=noprint_wrappers=1", normalizedPath,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("probe normalized video: %v: %s", err, probe)
	}
	probeText := string(probe)
	for _, want := range []string{"codec_name=h264", "pix_fmt=yuv420p", "codec_name=aac"} {
		if !strings.Contains(probeText, want) {
			t.Fatalf("normalized video missing %q:\n%s", want, probeText)
		}
	}
}

func TestTelegramVideoSanitizeCacheKeyUsesFileIdentity(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "video-*.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString("first"); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	first, ext, err := telegramVideoSanitizeCacheKey(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if ext != ".mp4" || first == "" {
		t.Fatalf("unexpected key=%q ext=%q", first, ext)
	}
	if err = os.WriteFile(file.Name(), []byte("second-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(time.Second)
	if err = os.Chtimes(file.Name(), now, now); err != nil {
		t.Fatal(err)
	}
	second, _, err := telegramVideoSanitizeCacheKey(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("cache key must change when the source file changes")
	}
}

func TestCloneTelegramVideoSanitizeFile(t *testing.T) {
	cachePath := t.TempDir() + "/cached.mp4"
	if err := os.WriteFile(cachePath, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	clonePath, err := cloneTelegramVideoSanitizeFile(cachePath, ".mp4")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(clonePath)
	content, err := os.ReadFile(clonePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "video" {
		t.Fatalf("unexpected clone content %q", content)
	}
}
