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
