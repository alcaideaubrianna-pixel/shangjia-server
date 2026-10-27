package storager

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"hotgo/internal/model"
)

func TestUploadEndpointEnv(t *testing.T) {
	for _, endpoint := range []string{
		"oss-ap-southeast-1-internal.aliyuncs.com",
		"https://oss-ap-southeast-1-internal.aliyuncs.com/",
		"xiaohuiji-tg-os.oss-ap-southeast-1-internal.aliyuncs.com",
	} {
		t.Run(endpoint, func(t *testing.T) {
			t.Setenv("YOUBAN_STORAGE_OSS_ENDPOINT", endpoint)
			original := &model.UploadConfig{OssBucket: "xiaohuiji-tg-os", OssEndpoint: "https://old.example", OssBucketURL: "https://cdn.example"}
			got, err := ApplyUploadEndpointEnv(original)
			if err != nil || got.OssEndpoint != "https://oss-ap-southeast-1-internal.aliyuncs.com" || got.OssBucketURL != original.OssBucketURL || original.OssEndpoint != "https://old.example" {
				t.Fatalf("override=%+v err=%v", got, err)
			}
		})
	}
}

func TestUploadEndpointEnvFallbackAndValidation(t *testing.T) {
	c := &model.UploadConfig{OssEndpoint: "https://saved.example"}
	t.Setenv("YOUBAN_STORAGE_OSS_ENDPOINT", " ")
	if got, err := ApplyUploadEndpointEnv(c); err != nil || got != c {
		t.Fatalf("database fallback=%v err=%v", got, err)
	}
	for _, endpoint := range []string{"https://host/path", "https://host?key=x", "ftp://host", "https://user:secret@host"} {
		t.Setenv("YOUBAN_STORAGE_OSS_ENDPOINT", endpoint)
		if _, err := ApplyUploadEndpointEnv(c); err == nil {
			t.Fatalf("accepted invalid endpoint %q", endpoint)
		}
	}
}

func TestUploadEndpointEnvKeepsBrowserUploadPublic(t *testing.T) {
	t.Setenv("YOUBAN_STORAGE_OSS_ENDPOINT", "xiaohuiji-tg-os.oss-ap-southeast-1-internal.aliyuncs.com")
	c, err := ApplyUploadEndpointEnv(&model.UploadConfig{OssBucket: "xiaohuiji-tg-os", OssSecretId: "test", OssSecretKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	previous := config
	t.Cleanup(func() { config = previous })
	SetConfig(c)
	signed, err := OssPresignedPutObject(context.Background(), "test.jpg", "image/jpeg", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(signed)
	if err != nil || strings.Contains(u.Host, "internal") || u.Host != "xiaohuiji-tg-os.oss-ap-southeast-1.aliyuncs.com" {
		t.Fatal("browser upload must use public OSS endpoint")
	}
}
