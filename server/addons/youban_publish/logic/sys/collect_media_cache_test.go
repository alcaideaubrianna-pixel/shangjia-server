package sys

import (
	"errors"
	"testing"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/internal/model/entity"
)

func TestDefaultCollectMediaFileDownloadTimeout(t *testing.T) {
	tests := []struct {
		name string
		size int64
		want time.Duration
	}{
		{name: "small image", size: 2 << 20, want: time.Minute},
		{name: "medium video", size: 20 << 20, want: 2 * time.Minute},
		{name: "large video", size: 80 << 20, want: 3 * time.Minute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := defaultCollectMediaFileDownloadTimeout(test.size); got != test.want {
				t.Fatalf("defaultCollectMediaFileDownloadTimeout(%d) = %s, want %s", test.size, got, test.want)
			}
		})
	}
}

func TestCollectMediaFileReferenceRefreshable(t *testing.T) {
	if !collectMediaFileReferenceRefreshable(errors.New("rpc error: FILE_REFERENCE_EXPIRED")) {
		t.Fatal("expected expired file reference to be refreshable")
	}
	if !collectMediaFileReferenceRefreshable(errors.New("rpc error: file_reference_invalid")) {
		t.Fatal("expected invalid file reference to be refreshable")
	}
	if collectMediaFileReferenceRefreshable(errors.New("rpc error: FILE_MIGRATE")) {
		t.Fatal("did not expect FILE_MIGRATE to be refreshable")
	}
	if retryErr := collectMediaRetryErrorFrom(errors.New("rpc error: FILE_REFERENCE_EXPIRED")); retryErr != nil {
		t.Fatalf("expired file reference must not use account retry: %+v", retryErr)
	}
}

func TestCollectMediaSourceGone(t *testing.T) {
	err := gerror.Wrap(errCollectMediaSourceGone, "TG原消息不存在或无权读取")
	if !collectMediaSourceGone(err) {
		t.Fatal("expected source-gone error to be detected")
	}
	if collectMediaSourceGone(errors.New("FILE_REFERENCE_INVALID")) {
		t.Fatal("file reference invalid should be refreshable, not source-gone")
	}
}

func TestGotdMediaCacheAssetKeyIgnoresFileReference(t *testing.T) {
	first := gotdMediaCacheAssetKey(gotdCollectMediaMeta{Kind: "photo", Id: 1, AccessHash: 2, FileReference: []byte("old"), Size: 3})
	second := gotdMediaCacheAssetKey(gotdCollectMediaMeta{Kind: "photo", Id: 1, AccessHash: 2, FileReference: []byte("new"), Size: 3})
	if first != second {
		t.Fatalf("cache asset key changed with file reference: %q != %q", first, second)
	}
}

func TestCollectMediaErrorIsRateLimited(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "localized message", message: "TG媒体下载触发限流", want: true},
		{name: "too many requests", message: "rpc error: TOO MANY REQUESTS", want: true},
		{name: "flood wait", message: "FLOOD_WAIT_30", want: true},
		{name: "network timeout", message: "context deadline exceeded", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectMediaErrorIsRateLimited(test.message); got != test.want {
				t.Fatalf("collectMediaErrorIsRateLimited(%q) = %v, want %v", test.message, got, test.want)
			}
		})
	}
}

func TestCollectTelegramMediaCacheSource(t *testing.T) {
	item := collectMediaItem{FileId: "gotd:-100123:456"}
	if got := collectTelegramMediaCacheSource(item, 1); got != "gotd:global:gotd:-100123:456" {
		t.Fatalf("collectTelegramMediaCacheSource() = %q", got)
	}
	item.FileId = "bot-file-id"
	if got := collectTelegramMediaCacheSource(item, 7); got != "gotd:account:7:bot-file-id" {
		t.Fatalf("collectTelegramMediaCacheSource() fallback = %q", got)
	}
}

func TestCollectMediaRowNeedsCacheSupportsMessageReference(t *testing.T) {
	if !collectMediaRowNeedsCache("", "gotd:-100123:456", "", "", "", 0) {
		t.Fatal("expected source message reference to trigger media cache")
	}
	if collectMediaRowNeedsCache("", "gotd:-100123:456", "storage/cache/media.jpg", "", "", 0) {
		t.Fatal("did not expect an existing storage path to trigger media cache")
	}
}

func TestCollectMediaRowsToItemsUsesMessageReference(t *testing.T) {
	items := collectMediaRowsToItems([]*collectEventMediaRow{{YoubanPublishCollectEventMedia: &entity.YoubanPublishCollectEventMedia{
		SourceMessageRef: "gotd:-100123:456", MediaType: "photo",
	}}}, "display")
	if len(items) != 1 || items[0].FileId != "gotd:-100123:456" {
		t.Fatalf("unexpected media items: %+v", items)
	}
}
