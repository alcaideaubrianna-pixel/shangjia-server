package storager

import (
	"hotgo/internal/model"
	"testing"
)

func TestR2DriveRequiresEndpointAndBucket(t *testing.T) {
	old := config
	t.Cleanup(func() { config = old })
	SetConfig(&model.UploadConfig{R2AccessKey: "a", R2SecretKey: "b"})
	if _, err := r2Options(); err == nil {
		t.Fatal("expected missing endpoint/bucket error")
	}
}

func TestR2RegionDefaultsToAuto(t *testing.T) {
	old := config
	t.Cleanup(func() { config = old })
	SetConfig(&model.UploadConfig{R2Endpoint: "127.0.0.1:1", R2Bucket: "bucket"})
	opts, err := r2Options()
	if err != nil || opts.Region != "auto" {
		t.Fatalf("region=%q err=%v", opts.Region, err)
	}
}
