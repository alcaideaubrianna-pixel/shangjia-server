package sys

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestMediaSourceMD5UsesOriginalVideo(t *testing.T) {
	original := []byte("original video bytes, not its poster")
	path := filepath.Join(t.TempDir(), "original.mp4")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := mediaSourceMD5(context.Background(), path, "https://invalid.example/poster.jpg", "video")
	if err != nil {
		t.Fatal(err)
	}
	want := md5.Sum(original)
	if got != hex.EncodeToString(want[:]) {
		t.Fatalf("MD5=%s, want original digest", got)
	}
}

func TestMediaSourceMD5RejectsMissingOriginal(t *testing.T) {
	if _, err := mediaSourceMD5(context.Background(), "", "", "video"); err == nil {
		t.Fatal("missing original must not silently produce an empty MD5")
	}
}
