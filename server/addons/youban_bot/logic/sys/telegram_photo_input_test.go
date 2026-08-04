package sys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePublicMediaPath(t *testing.T) {
	root := t.TempDir()
	imagePath := filepath.Join(root, "attachment", "welcome.jpg")
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{
		"attachment/welcome.jpg",
		"/attachment/welcome.jpg",
		"https://localhost:8000/attachment/welcome.jpg",
		imagePath,
	} {
		if got := resolvePublicMediaPath(root, source); got != imagePath {
			t.Fatalf("resolvePublicMediaPath(%q)=%q, want %q", source, got, imagePath)
		}
	}
}

func TestResolvePublicMediaPathRejectsInvalidFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.jpg")
	if err := os.WriteFile(outside, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })

	for _, source := range []string{"attachment/missing.jpg", outside, "../outside.jpg"} {
		if got := resolvePublicMediaPath(root, source); got != "" {
			t.Fatalf("resolvePublicMediaPath(%q)=%q, want empty", source, got)
		}
	}
}
