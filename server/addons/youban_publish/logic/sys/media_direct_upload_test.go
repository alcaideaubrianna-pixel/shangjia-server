package sys

import (
	"strings"
	"testing"

	"hotgo/internal/library/storager"
)

func TestDirectUploadBucketRegion(t *testing.T) {
	bucket, region, err := directUploadBucketRegion("https://youbanyue01-1442821378.cos.ap-hongkong.myqcloud.com")
	if err != nil {
		t.Fatalf("directUploadBucketRegion returned error: %v", err)
	}
	if bucket != "youbanyue01-1442821378" || region != "ap-hongkong" {
		t.Fatalf("unexpected bucket or region: %s %s", bucket, region)
	}
}

func TestDirectUploadObjectKeyKeepsExtensionDot(t *testing.T) {
	key := storager.GenFullPath("attachment/", "."+storager.Ext("example.PNG"))
	if !strings.HasSuffix(key, ".png") {
		t.Fatalf("expected COS object key to end with .png, got %q", key)
	}
}

func TestDirectUploadMultipartQueryHelpers(t *testing.T) {
	query := map[string][]string{
		"prefix":  {"hotgov1/attachment/video.mov"},
		"uploads": {""},
	}
	if actual := directUploadQueryValue(query, "prefix"); actual != "hotgov1/attachment/video.mov" {
		t.Fatalf("unexpected prefix: %q", actual)
	}
	if !directUploadQueryHasKey(query, "uploads") {
		t.Fatal("expected uploads query key")
	}
	if directUploadQueryValue(query, "missing") != "" {
		t.Fatal("unexpected missing query value")
	}
}

func TestDirectUploadBucketRegionRejectsInvalidURL(t *testing.T) {
	if _, _, err := directUploadBucketRegion("https://example.com"); err == nil {
		t.Fatal("expected invalid COS URL to be rejected")
	}
}

func TestDirectUploadBucketRegionFromConfigAllowsCDNPublicURL(t *testing.T) {
	bucket, region, err := directUploadBucketRegionFromConfig(
		"",
		"",
		"https://youbanyue01-1442821378.cos.ap-hongkong.myqcloud.com",
		"https://img.yuebanby.com",
	)
	if err != nil {
		t.Fatalf("directUploadBucketRegionFromConfig returned error: %v", err)
	}
	if bucket != "youbanyue01-1442821378" || region != "ap-hongkong" {
		t.Fatalf("unexpected bucket/region: %q %q", bucket, region)
	}
}

func TestDirectUploadBucketRegionFromConfigSupportsLegacySwappedURLs(t *testing.T) {
	bucket, region, err := directUploadBucketRegionFromConfig(
		"",
		"",
		"https://img.yuebanby.com",
		"https://youbanyue01-1442821378.cos.ap-hongkong.myqcloud.com",
	)
	if err != nil || bucket != "youbanyue01-1442821378" || region != "ap-hongkong" {
		t.Fatalf("unexpected result: bucket=%q region=%q err=%v", bucket, region, err)
	}
}

func TestDirectUploadBucketRegionFromExplicitConfig(t *testing.T) {
	bucket, region, err := directUploadBucketRegionFromConfig(
		"youbanyue01-1442821378",
		"ap-hongkong",
		"https://img.yuebanby.com",
		"https://img.yuebanby.com",
	)
	if err != nil || bucket != "youbanyue01-1442821378" || region != "ap-hongkong" {
		t.Fatalf("unexpected result: bucket=%q region=%q err=%v", bucket, region, err)
	}
}

func TestDirectUploadExtensionUsesStoragerFormat(t *testing.T) {
	if ext := storager.Ext("example.PNG"); ext != "png" || !storager.IsImgType(ext) {
		t.Fatalf("expected PNG extension to pass image validation, got %q", ext)
	}
	if ext := storager.Ext("example.MP4"); ext != "mp4" || !storager.IsVideoType(ext) {
		t.Fatalf("expected MP4 extension to pass video validation, got %q", ext)
	}
}
