package sys

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"testing"
)

func encodeBackgroundReplacementTestImage(t *testing.T, fill color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, fill)
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestShouldSkipBackgroundReplacementTable(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	for _, y := range []int{0, 40, 80, 120, 160, 200, 239} {
		for x := 0; x < 320; x++ {
			img.Set(x, y, color.Gray{Y: 80})
		}
	}
	for _, x := range []int{0, 106, 212, 319} {
		for y := 0; y < 240; y++ {
			img.Set(x, y, color.Gray{Y: 80})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if !shouldSkipBackgroundReplacement(out.Bytes()) {
		t.Fatal("table image should skip cloud portrait matting")
	}
}

func TestShouldNotSkipWhiteStudioPortrait(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 240))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(55, 25, 145, 230), &image.Uniform{C: color.RGBA{R: 56, G: 122, B: 186, A: 255}}, image.Point{}, draw.Src)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if shouldSkipBackgroundReplacement(out.Bytes()) {
		t.Fatal("white studio portrait should remain eligible for matting")
	}
}

func TestDocumentImageSkipsAllAntiScanProcessing(t *testing.T) {
	data := encodeBackgroundReplacementTestImage(t, color.White)
	source, err := os.CreateTemp("", "ybp-document-*.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(source.Name())
	if _, err = source.Write(data); err != nil {
		t.Fatal(err)
	}
	if err = source.Close(); err != nil {
		t.Fatal(err)
	}
	media := &telegramMediaItem{
		MediaType: "image", AntiScanEnabled: true,
		AntiScanMode: "background_replace", AntiScanBackgroundURL: "https://example.com/background.jpg",
	}
	path, cleanup, err := NewSysPublish().prepareTelegramAntiScanUploadFile(t.Context(), media, source.Name(), nil, "image")
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		t.Fatal(err)
	}
	if path != source.Name() {
		t.Fatalf("document must keep original file, got %q want %q", path, source.Name())
	}
}

func TestShouldSkipBackgroundReplacementWhiteDocument(t *testing.T) {
	if !shouldSkipBackgroundReplacement(encodeBackgroundReplacementTestImage(t, color.White)) {
		t.Fatal("white document should skip cloud portrait matting")
	}
}

func TestShouldSkipBackgroundReplacementPhotoLikeImage(t *testing.T) {
	if shouldSkipBackgroundReplacement(encodeBackgroundReplacementTestImage(t, color.RGBA{R: 73, G: 128, B: 182, A: 255})) {
		t.Fatal("photo-like image should remain eligible for portrait matting")
	}
}
