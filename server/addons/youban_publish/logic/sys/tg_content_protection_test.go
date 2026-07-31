package sys

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTelegramAntiScanCacheKeyRemainsStableAfterMarking(t *testing.T) {
	media := &telegramMediaItem{Id: 12, Purpose: "display", AssetHash: "source-hash", AntiScanSeed: 34}
	key := telegramAntiScanCacheKey(media)
	media.AssetHash = "anti-scan:" + key
	if got := telegramAntiScanCacheKey(media); got != key {
		t.Fatalf("cache key changed after marking: got %q want %q", got, key)
	}
}

func TestRenderTelegramAntiScanFileDeterministicBySeed(t *testing.T) {
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source.png")
	writeAntiScanTestImage(t, sourcePath)

	firstPath := filepath.Join(dir, "first.jpg")
	secondPath := filepath.Join(dir, "second.jpg")
	thirdPath := filepath.Join(dir, "third.jpg")
	if err := renderTelegramAntiScanFile(sourcePath, firstPath, 101); err != nil {
		t.Fatal(err)
	}
	if err := renderTelegramAntiScanFile(sourcePath, secondPath, 101); err != nil {
		t.Fatal(err)
	}
	if err := renderTelegramAntiScanFile(sourcePath, thirdPath, 202); err != nil {
		t.Fatal(err)
	}
	first := mustReadFile(t, firstPath)
	second := mustReadFile(t, secondPath)
	third := mustReadFile(t, thirdPath)
	if !bytes.Equal(first, second) {
		t.Fatal("same seed should generate identical retry output")
	}
	if bytes.Equal(first, third) {
		t.Fatal("different push seeds should generate different image output")
	}
}

func TestObfuscateTelegramCaptionKeepsHTMLAndLimitsNumberScope(t *testing.T) {
	caption := `<b>广州资料</b>\n📏身高：174\n⚖️体重：105kg\n电话：13800138000\n编号：G35535`
	first := obfuscateTelegramCaption(caption, 88)
	second := obfuscateTelegramCaption(caption, 88)
	if first != second {
		t.Fatal("same job seed should generate identical retry caption")
	}
	if !strings.Contains(first, "<b>广州资料</b>") {
		t.Fatalf("telegram HTML markup was not preserved: %s", first)
	}
	if !strings.Contains(first, "13800138000") || !strings.Contains(first, "G35535") {
		t.Fatalf("unrelated numbers were modified: %s", first)
	}
	if strings.Contains(first, "身高：174") || strings.Contains(first, "体重：105kg") {
		t.Fatalf("height or weight was not perturbed: %s", first)
	}
	if !regexp.MustCompile(`身高[^0-9]{0,8}17[3-4]\.[0-9]`).MatchString(first) {
		t.Fatalf("height perturbation exceeded expected range: %s", first)
	}
	if !regexp.MustCompile(`体重[^0-9]{0,8}10[4-5]\.[0-9]kg`).MatchString(first) {
		t.Fatalf("weight perturbation exceeded expected range: %s", first)
	}
}

func writeAntiScanTestImage(t *testing.T, path string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 96, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 96; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 2), G: uint8(y * 2), B: uint8((x + y) % 255), A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, img); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
