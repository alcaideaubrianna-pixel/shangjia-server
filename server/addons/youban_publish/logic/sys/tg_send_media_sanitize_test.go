package sys

import (
	"image"
	"testing"
)

func TestNormalizeTelegramPhotoDimensions(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "large square", width: 8000, height: 8000},
		{name: "wide banner", width: 12000, height: 300},
		{name: "tall banner", width: 300, height: 12000},
		{name: "valid photo", width: 1920, height: 1080},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := image.NewRGBA(image.Rect(0, 0, tt.width, tt.height))
			out := normalizeTelegramPhotoDimensions(img)
			if out == nil {
				t.Fatal("expected image")
			}
			bounds := out.Bounds()
			if !telegramPhotoDimensionsValid(bounds.Dx(), bounds.Dy()) {
				t.Fatalf("invalid dimensions after normalization: %dx%d", bounds.Dx(), bounds.Dy())
			}
		})
	}
}
