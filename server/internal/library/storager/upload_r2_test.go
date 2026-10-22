package storager

import (
	"testing"

	"hotgo/internal/model"
)

func TestR2DriveRequiresEndpointAndBucket(t *testing.T) {
	old := config
	t.Cleanup(func() { config = old })
	SetConfig(&model.UploadConfig{R2AccessKey: "a", R2SecretKey: "b"})
	if _, _, err := r2ClientConfig(); err == nil {
		t.Fatal("expected missing endpoint/bucket error")
	}
}

func TestR2RegionDefaultsToAuto(t *testing.T) {
	old := config
	t.Cleanup(func() { config = old })
	SetConfig(&model.UploadConfig{R2Endpoint: "127.0.0.1:1", R2Bucket: "bucket", R2AccessKey: "access", R2SecretKey: "secret"})
	_, opts, err := r2ClientConfig()
	if err != nil || opts.Region != "auto" {
		t.Fatalf("region=%q err=%v", opts.Region, err)
	}
}

func TestR2EndpointAcceptsHTTPSURL(t *testing.T) {
	old := config
	t.Cleanup(func() { config = old })
	SetConfig(&model.UploadConfig{
		R2Endpoint:  "https://account.r2.cloudflarestorage.com/",
		R2Bucket:    "bucket",
		R2AccessKey: "access",
		R2SecretKey: "secret",
	})
	endpoint, opts, err := r2ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != "account.r2.cloudflarestorage.com" || !opts.Secure {
		t.Fatalf("endpoint=%q secure=%v", endpoint, opts.Secure)
	}
}

func TestR2EndpointRejectsPath(t *testing.T) {
	old := config
	t.Cleanup(func() { config = old })
	SetConfig(&model.UploadConfig{R2Endpoint: "https://example.com/path", R2Bucket: "bucket", R2AccessKey: "access", R2SecretKey: "secret"})
	_, _, err := r2ClientConfig()
	if err == nil {
		t.Fatal("expected invalid endpoint error")
	}
}
