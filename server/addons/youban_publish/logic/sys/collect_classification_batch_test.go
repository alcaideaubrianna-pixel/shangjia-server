package sys

import (
	"context"
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestCollectBatchClassificationUsesLoadedMedia(t *testing.T) {
	// No DB is configured here: nil/empty media are authoritative batch results.
	for _, tc := range []struct {
		name, text string
		media      []collectMediaItem
	}{
		{"no_media", "这是一份超过二十个字的展示资料文案，但本条消息没有媒体", nil},
		{"empty_media", "", []collectMediaItem{}},
		{"display", "这是一份超过二十个字的展示资料文案，附带图片展示信息", []collectMediaItem{{Type: "photo", FileId: "photo"}}},
		{"verify", "Q48201", []collectMediaItem{{Type: "video", FileId: "video"}}},
		{"mixed_album", "Q48201", []collectMediaItem{{Type: "photo"}, {Type: "video"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := gdb.Record{"id": gvar.New(1), "raw_text": gvar.New(tc.text)}
			got, err := classifyCollectEventMedia(context.Background(), event, tc.text, tc.media)
			if err != nil {
				t.Fatal(err)
			}
			want := classifyProfileMessage(tc.text, tc.media)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("classification changed: got=%+v want=%+v", got, want)
			}
		})
	}
}
