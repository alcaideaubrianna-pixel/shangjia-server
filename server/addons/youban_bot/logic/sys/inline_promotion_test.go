package sys

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestNormalizeInlinePromotionStartParameter(t *testing.T) {
	if got := normalizeInlinePromotionStartParameter("promo_2026-test"); got != "promo_2026-test" {
		t.Fatalf("unexpected valid parameter: %q", got)
	}
	for _, value := range []string{"", "with space", "中文", "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789___"} {
		if got := normalizeInlinePromotionStartParameter(value); got != "inline_entry" {
			t.Fatalf("invalid parameter %q should use fallback, got %q", value, got)
		}
	}
}

func TestInlinePromotionFeatureHidesMenuEntry(t *testing.T) {
	schema := (inlinePromotionFeature{}).ConfigSchema()
	if len(schema) == 0 || schema[0].Field != "menuVisible" || schema[0].Component != "hidden" || schema[0].Default != 0 {
		t.Fatalf("inline promotion menu configuration is not hidden: %+v", schema)
	}
}

func TestInlinePromotionLegacyMenuLabel(t *testing.T) {
	feature := inlinePromotionFeature{}
	if !feature.Match(context.Background(), nil, nil, " 合作推广广告 ") {
		t.Fatal("legacy promotion menu label should match")
	}
	if feature.Match(context.Background(), nil, nil, "合作推广") {
		t.Fatal("unrelated promotion text should not match")
	}
}

func TestInlinePromotionTargetFeatureOptions(t *testing.T) {
	options := inlinePromotionTargetFeatureOptions()
	foundStart := false
	for _, option := range options {
		if option.Value == inlinePromotionFeatureKey {
			t.Fatal("inline promotion must not target itself")
		}
		if option.Value == (startFeature{}).Key() {
			foundStart = true
		}
	}
	if !foundStart {
		t.Fatal("start feature option is missing")
	}
	if got := normalizeInlinePromotionTargetFeatureKey("unknown"); got != (startFeature{}).Key() {
		t.Fatalf("unknown target should fall back to start, got %q", got)
	}
}

func TestBuildInlinePromotionImageAssets(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	for y := 0; y < 720; y++ {
		for x := 0; x < 1280; x++ {
			source.SetRGBA(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	buffer := bytes.NewBuffer(nil)
	if err := jpeg.Encode(buffer, source, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	assets, err := buildInlinePromotionImageAssets(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if assets.Width != 1280 || assets.Height != 720 || len(assets.MainImage) != 0 {
		t.Fatalf("unexpected main image result: %+v", assets)
	}
	thumbnailConfig, format, err := image.DecodeConfig(bytes.NewReader(assets.ThumbnailImage))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" || thumbnailConfig.Width != 640 || thumbnailConfig.Height != 640 {
		t.Fatalf("unexpected thumbnail: format=%s size=%dx%d", format, thumbnailConfig.Width, thumbnailConfig.Height)
	}
}

func TestBuildInlinePromotionImageAssetsRejectsSmallImage(t *testing.T) {
	buffer := bytes.NewBuffer(nil)
	if err := jpeg.Encode(buffer, image.NewRGBA(image.Rect(0, 0, 320, 180)), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if _, err := buildInlinePromotionImageAssets(buffer.Bytes()); err == nil {
		t.Fatal("small image should be rejected")
	}
}

func TestBuildInlinePromotionImageAssetsResizesLargeImage(t *testing.T) {
	buffer := bytes.NewBuffer(nil)
	if err := jpeg.Encode(buffer, image.NewRGBA(image.Rect(0, 0, 3000, 1688)), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	assets, err := buildInlinePromotionImageAssets(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if assets.Width != 2560 || assets.Height != 1440 || len(assets.MainImage) == 0 {
		t.Fatalf("large image was not resized as expected: width=%d height=%d bytes=%d", assets.Width, assets.Height, len(assets.MainImage))
	}
}
