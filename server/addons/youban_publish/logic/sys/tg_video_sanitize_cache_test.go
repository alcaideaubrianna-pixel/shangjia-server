package sys

import (
	"os"
	"testing"
	"time"
)

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
