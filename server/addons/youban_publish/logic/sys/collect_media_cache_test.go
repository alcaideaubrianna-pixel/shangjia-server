package sys

import (
	"errors"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/internal/model/entity"
)

func TestCollectMediaSourceGone(t *testing.T) {
	err := gerror.Wrap(errCollectMediaSourceGone, "TG原消息不存在或无权读取")
	if !collectMediaSourceGone(err) {
		t.Fatal("expected source-gone error to be detected")
	}
	if collectMediaSourceGone(errors.New("FILE_REFERENCE_INVALID")) {
		t.Fatal("file reference invalid should be refreshable, not source-gone")
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

func TestCollectMediaRowNeedsCacheSupportsMessageReference(t *testing.T) {
	if !collectMediaRowNeedsCache("", "gotd:-100123:456", "", "", "", 0) {
		t.Fatal("expected source message reference to trigger media cache")
	}
	if collectMediaRowNeedsCache("", "gotd:-100123:456", "storage/cache/media.jpg", "", "", 0) {
		t.Fatal("did not expect an existing storage path to trigger media cache")
	}
}

func TestCollectEventMediaRowNeedsCacheSupportsBotFileID(t *testing.T) {
	row := &collectEventMediaRow{YoubanPublishCollectEventMedia: &entity.YoubanPublishCollectEventMedia{
		SourceFileId: "AgACBotFileID",
		MediaType:    "photo",
	}}
	if !collectEventMediaRowNeedsCache(gdb.Record{"source_type": gvar.New("bot")}, row) {
		t.Fatal("expected a Bot file ID without a local path to require media cache")
	}
	if collectEventMediaRowNeedsCache(gdb.Record{"source_type": gvar.New("account")}, row) {
		t.Fatal("did not expect an account file ID without gotd metadata to require media cache")
	}
	row.StoragePath = "storage/cache/media.jpg"
	if collectEventMediaRowNeedsCache(gdb.Record{"source_type": gvar.New("bot")}, row) {
		t.Fatal("did not expect a cached Bot media row to require media cache")
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
