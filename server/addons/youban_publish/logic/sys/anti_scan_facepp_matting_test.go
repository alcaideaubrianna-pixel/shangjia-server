package sys

import (
	"bytes"
	"crypto/rand"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"
)

func TestPrepareFacePPMattingImageBytesUsesLosslessPNGFirst(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1024; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 18, G: 52, B: 86, A: 255})
		}
	}
	var source bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.NoCompression}
	if err := encoder.Encode(&source, img); err != nil {
		t.Fatal(err)
	}
	if source.Len() <= facePPMaximumImageBytes {
		t.Fatalf("test image must exceed Face++ limit: %d", source.Len())
	}
	output, contentType, filename, err := prepareFacePPMattingImageBytes(source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "image/png" || filename != "image.png" {
		t.Fatalf("expected lossless PNG, got type=%q filename=%q", contentType, filename)
	}
	if len(output) > facePPMaximumImageBytes {
		t.Fatalf("compressed PNG still exceeds limit: %d", len(output))
	}
}

func TestPrepareFacePPMattingImageBytesFallsBackToJPEG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1200, 1200))
	if _, err := rand.Read(img.Pix); err != nil {
		t.Fatal(err)
	}
	var source bytes.Buffer
	if err := png.Encode(&source, img); err != nil {
		t.Fatal(err)
	}
	output, contentType, filename, err := prepareFacePPMattingImageBytes(source.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "image/jpeg" || filename != "image.jpg" {
		t.Fatalf("expected JPEG fallback, got type=%q filename=%q", contentType, filename)
	}
	if len(output) > facePPMaximumImageBytes {
		t.Fatalf("JPEG still exceeds limit: %d", len(output))
	}
	if _, _, err = image.Decode(bytes.NewReader(output)); err != nil {
		t.Fatalf("JPEG output cannot be decoded: %v", err)
	}
}

func TestFacePPPermanentResponseClassification(t *testing.T) {
	for _, tc := range []struct {
		status  int
		message string
		want    bool
	}{
		{http.StatusRequestEntityTooLarge, "", true},
		{http.StatusBadRequest, "IMAGE_FILE_TOO_LARGE:image_file", true},
		{http.StatusUnauthorized, "AUTHENTICATION_ERROR", true},
		{http.StatusTooManyRequests, "CONCURRENCY_LIMIT_EXCEEDED", false},
		{http.StatusInternalServerError, "INTERNAL_ERROR", false},
	} {
		err := &facePPAPIError{StatusCode: tc.status, Message: tc.message}
		if got := errors.Is(err, errFacePPPermanent); got != tc.want {
			t.Fatalf("status=%d message=%q permanent=%v want=%v", tc.status, tc.message, got, tc.want)
		}
	}
}
