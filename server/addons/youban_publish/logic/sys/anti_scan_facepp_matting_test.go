package sys

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"strings"
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

// Face++ returns the transparent image as Base64 in JSON, not as the small
// source JPEG. This response reproduces the production 4 MiB truncation.
func TestReadFacePPMattingResponseAboveFourMiB(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1024, 1024))
	copy(img.Pix, bytes.Repeat([]byte{18, 52, 86, 128}, 1024*1024))
	var pngBody bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.NoCompression}
	if err := encoder.Encode(&pngBody, img); err != nil {
		t.Fatal(err)
	}
	imageBytes := pngBody.Bytes()
	response, err := json.Marshal(map[string]string{"body_image": base64.StdEncoding.EncodeToString(imageBytes)})
	if err != nil {
		t.Fatal(err)
	}
	if len(response) <= 4*1024*1024 {
		t.Fatal("fixture must exceed former response limit")
	}
	raw, err := readFacePPMattingResponse(bytes.NewReader(response))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		BodyImage string `json:"body_image"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("valid large response was truncated: %v", err)
	}
	decoded, err := decodeBase64Image(out.BodyImage)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, imageBytes) {
		t.Fatal("returned image differs from provider response")
	}
}

func TestReadFacePPMattingResponsePreservesReadFailure(t *testing.T) {
	_, err := readFacePPMattingResponse(io.MultiReader(strings.NewReader(`{"body_image":"`), facePPFailingReader{}))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("lost upstream read error: %v", err)
	}
}

type facePPFailingReader struct{}

func (facePPFailingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
