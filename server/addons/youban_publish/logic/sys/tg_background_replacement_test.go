package sys

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
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
