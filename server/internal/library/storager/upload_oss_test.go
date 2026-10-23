package storager

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"hotgo/internal/consts"
	"hotgo/internal/model"
)

func TestOssPublicEndpointConvertsInternalEndpoint(t *testing.T) {
	got, err := OssPublicEndpoint("https://oss-ap-southeast-1-internal.aliyuncs.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://oss-ap-southeast-1.aliyuncs.com" {
		t.Fatalf("unexpected public endpoint: %q", got)
	}
}

func TestOssPublicEndpointAddsHTTPS(t *testing.T) {
	got, err := OssPublicEndpoint("oss-cn-hangzhou-internal.aliyuncs.com")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://oss-cn-hangzhou.aliyuncs.com" {
		t.Fatalf("unexpected public endpoint: %q", got)
	}
}

func TestOssPublicEndpointRejectsInvalidEndpoint(t *testing.T) {
	for _, endpoint := range []string{"", "ftp://oss.example.com", "https://oss.example.com/path"} {
		if _, err := OssPublicEndpoint(endpoint); err == nil {
			t.Fatalf("expected endpoint %q to be rejected", endpoint)
		}
	}
}

func TestOssLastURLNormalizesSlashes(t *testing.T) {
	old := config
	t.Cleanup(func() { config = old })
	SetConfig(&model.UploadConfig{OssBucketURL: "https://img.example.com/"})
	got := LastUrl(context.Background(), "/hotgo/file/image.jpg", consts.UploadDriveOss)
	if got != "https://img.example.com/hotgo/file/image.jpg" {
		t.Fatalf("unexpected OSS URL: %q", got)
	}
}

func TestOssPresignedPutObjectUsesPublicEndpoint(t *testing.T) {
	old := config
	t.Cleanup(func() { config = old })
	SetConfig(&model.UploadConfig{
		OssBucket:    "media-bucket",
		OssEndpoint:  "https://oss-ap-southeast-1-internal.aliyuncs.com",
		OssSecretId:  "access-key",
		OssSecretKey: "secret-key",
	})
	signedURL, err := OssPresignedPutObject(context.Background(), "hotgo/file/test.jpg", "image/jpeg", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(parsed.Host, "-internal.") || !strings.Contains(parsed.Host, "oss-ap-southeast-1.aliyuncs.com") {
		t.Fatalf("signed URL does not use the public OSS endpoint: %q", signedURL)
	}
	if parsed.Query().Get("OSSAccessKeyId") == "" || parsed.Query().Get("Signature") == "" {
		t.Fatalf("signed URL is missing OSS authorization parameters: %q", signedURL)
	}
}
