package sys

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestPrepareFapiHubImageBytesResizesLargeImage(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 2400, 1200))
	source.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	var input bytes.Buffer
	if err := jpeg.Encode(&input, source, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}

	got, contentType, ext, err := prepareFapiHubImageBytes(input.Bytes(), 1200)
	if err != nil {
		t.Fatal(err)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 1200 || config.Height != 600 {
		t.Fatalf("unexpected dimensions: %dx%d", config.Width, config.Height)
	}
	if contentType != "image/jpeg" || ext != "jpg" {
		t.Fatalf("unexpected output type: %s .%s", contentType, ext)
	}
}

func TestFapiHubSegmentURLStorageFormat(t *testing.T) {
	const expected = "https://cdn.example.com/segment.png"
	raw := encodeFapiHubSegmentPortraitURL(expected)
	if got := antiScanSegmentURL(raw); got != expected {
		t.Fatalf("unexpected segment URL: %q", got)
	}
	if len(raw) >= len(encodeFapiHubSegmentPortrait(make([]byte, 1024))) {
		t.Fatal("URL storage should be smaller than legacy base64 storage")
	}
}

func TestLegacyFapiHubSegmentCanBeMigrated(t *testing.T) {
	expected := []byte("png-content")
	raw := encodeFapiHubSegmentPortrait(expected)
	if got := antiScanSegmentImageBytes(raw); !bytes.Equal(got, expected) {
		t.Fatalf("unexpected legacy segment bytes: %q", got)
	}
}
