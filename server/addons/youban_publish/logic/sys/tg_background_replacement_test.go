package sys

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
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

func TestAntiScanPortraitUnderlayMaskKeepsThreeDistinctLayers(t *testing.T) {
	bounds := image.Rect(0, 0, 200, 240)
	portrait := image.NewRGBA(bounds)
	draw.Draw(portrait, image.Rect(90, 80, 110, 220), &image.Uniform{C: color.RGBA{R: 20, G: 30, B: 40, A: 255}}, image.Point{}, draw.Src)

	mask := antiScanPortraitUnderlayMask(portrait, bounds)
	if alpha := mask.AlphaAt(10, 10).A; alpha != 0 {
		t.Fatalf("distant background must stay opaque and untouched, alpha=%d", alpha)
	}
	if alpha := mask.AlphaAt(60, 140).A; alpha == 0 || alpha == 255 {
		t.Fatalf("portrait margin must be feathered, alpha=%d", alpha)
	}
	if alpha := mask.AlphaAt(100, 140).A; alpha < 250 {
		t.Fatalf("original underlay around portrait must remain opaque, alpha=%d", alpha)
	}
}

func TestLosslessBackgroundReplacementPreservesPortraitPixels(t *testing.T) {
	bounds := image.Rect(0, 0, 80, 100)
	source := image.NewRGBA(bounds)
	draw.Draw(source, bounds, &image.Uniform{C: color.RGBA{R: 30, G: 50, B: 70, A: 255}}, image.Point{}, draw.Src)
	portrait := image.NewRGBA(bounds)
	draw.Draw(portrait, image.Rect(30, 20, 50, 90), &image.Uniform{C: color.RGBA{R: 217, G: 133, B: 91, A: 255}}, image.Point{}, draw.Src)
	var sourceBytes bytes.Buffer
	if err := jpeg.Encode(&sourceBytes, source, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	var portraitBytes bytes.Buffer
	if err := png.Encode(&portraitBytes, portrait); err != nil {
		t.Fatal(err)
	}
	segment, err := json.Marshal(map[string]any{"Response": map[string]string{
		"ResultImage": base64.StdEncoding.EncodeToString(portraitBytes.Bytes()),
	}})
	if err != nil {
		t.Fatal(err)
	}
	in := &sysin.AntiScanPreviewInp{}
	in.BackgroundReplaceEnabled = 1
	in.BackgroundTexturePreset = "dot"
	output, warnings, err := renderAntiScanPreviewLossless(context.Background(), sourceBytes.Bytes(), in, &antiScanDetectResult{SegmentRaw: string(segment)})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	result, format, err := image.Decode(bytes.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" {
		t.Fatalf("unexpected output format: %s", format)
	}
	want := portrait.RGBAAt(40, 50)
	got := color.RGBAModel.Convert(result.At(40, 50)).(color.RGBA)
	if got != want {
		t.Fatalf("portrait pixel changed after lossless render: got %#v want %#v", got, want)
	}
}
