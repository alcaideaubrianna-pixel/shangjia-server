package sys

import (
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestFindCollectVerifyEventUsesOneFollowingVideoGroup(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-4 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(1), "source_message_id": gvar.New(100), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A100"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(2), "source_message_id": gvar.New(101), "material_role": gvar.New("pending"), "raw_text": gvar.New("频道通知"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(3), "source_message_id": gvar.New(102), "material_role": gvar.New("pending"), "raw_text": gvar.New(""), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(4), "source_message_id": gvar.New(103), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A101"), "received_at": gvar.New(receivedAt)},
	}
	if got := (&sSysPublish{}).findCollectVerifyEvent(rows, collectMaterialEventViews(rows, collectTestMediaByEvent(rows)), 0); got != 2 {
		t.Fatalf("verify event index=%d, want 2", got)
	}
}

func TestFindCollectVerifyEventDoesNotCrossNextDisplayGroup(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-4 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(1), "source_message_id": gvar.New(100), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A100"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(2), "source_message_id": gvar.New(101), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A101"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(3), "source_message_id": gvar.New(102), "material_role": gvar.New("pending"), "raw_text": gvar.New(""), "received_at": gvar.New(receivedAt)},
	}
	if got := (&sSysPublish{}).findCollectVerifyEvent(rows, collectMaterialEventViews(rows, collectTestMediaByEvent(rows)), 0); got != -1 {
		t.Fatalf("verify event index=%d, want no cross-group match", got)
	}
}

func TestFindCollectDisplayEventRepairsAlreadyPairedVerify(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-4 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(10), "source_message_id": gvar.New(200), "material_role": gvar.New("display"), "material_parent_event_id": gvar.New(0), "raw_text": gvar.New("昵称：A200"), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(11), "source_message_id": gvar.New(201), "material_role": gvar.New("verify"), "material_parent_event_id": gvar.New(10), "status": gvar.New("ignored"), "error_message": gvar.New(collectMaterialVerifyUnmatchedMessage), "raw_text": gvar.New(""), "received_at": gvar.New(receivedAt)},
	}
	if got := (&sSysPublish{}).findCollectDisplayEvent(rows, collectMaterialEventViews(rows, collectTestMediaByEvent(rows)), 1); got != 0 {
		t.Fatalf("display event index=%d, want 0", got)
	}
	if !collectMaterialEventNeedsPairRepair(rows[1]) {
		t.Fatal("already ignored unmatched verify event should be repairable")
	}
}

func collectTestMediaByEvent(rows []gdb.Record) map[int64][]collectMediaItem {
	result := make(map[int64][]collectMediaItem, len(rows))
	for _, row := range rows {
		text := row["raw_text"].String()
		messageId := row["source_message_id"].Int64()
		switch {
		case text == "" && (messageId == 102 || messageId == 201):
			result[row["id"].Int64()] = []collectMediaItem{{Type: "video", FileId: "video"}}
		case strings.HasPrefix(text, "昵称："):
			result[row["id"].Int64()] = []collectMediaItem{{Type: "photo", FileId: "photo"}}
		}
	}
	return result
}

func TestCollectMaterialEventOlderThanUsesDatabaseWallClock(t *testing.T) {
	databaseTime := time.Now().Add(-4 * time.Minute).UTC()
	event := gdb.Record{"received_at": gvar.New(databaseTime)}
	if !collectMaterialEventOlderThan(event, 3*time.Minute) {
		t.Fatalf("database wall-clock event should be considered older than grouping window")
	}
}

func TestCollectMaterialEventIngestOlderThanUsesCreatedAt(t *testing.T) {
	event := gdb.Record{
		"received_at": gvar.New(time.Now().Add(-24 * time.Hour)),
		"created_at":  gvar.New(time.Now().Add(-time.Minute)),
	}
	if collectMaterialEventIngestOlderThan(event, 3*time.Minute) {
		t.Fatal("historical event should wait from ingestion time before finalizing an unmatched pair")
	}
}

func TestPairCollectMaterialMessagesCases(t *testing.T) {
	tests := []struct {
		name  string
		views []collectMaterialMessageView
		want  []collectMaterialPair
	}{
		{
			name: "display then verify",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}},
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 0, VerifyIndex: 1}},
		},
		{
			name: "notice between display and verify",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}},
				{RawText: "✅提交成功！"},
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 0, VerifyIndex: 2}},
		},
		{
			name: "repeated profile text between media display and verify",
			views: []collectMaterialMessageView{
				{RawText: "昵称：SKS457", Media: []collectMediaItem{{Type: "photo", FileId: "p"}, {Type: "video", FileId: "display-v"}}},
				{RawText: "昵称：SKS457\n年龄：20"},
				{Media: []collectMediaItem{{Type: "video", FileId: "verify-v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 0, VerifyIndex: 2}},
		},
		{
			name: "text only display can pair when no media display is waiting",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A\n年龄：20"},
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 0, VerifyIndex: 1}},
		},
		{
			name: "next display closes previous",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p1"}}},
				{RawText: "昵称：B", Media: []collectMediaItem{{Type: "photo", FileId: "p2"}}},
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
			},
			want: []collectMaterialPair{{DisplayIndex: 1, VerifyIndex: 2}},
		},
		{
			name: "verify before display is isolated",
			views: []collectMaterialMessageView{
				{Media: []collectMediaItem{{Type: "video", FileId: "v"}}},
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}},
			},
		},
		{
			name: "mixed media without text is not verify",
			views: []collectMaterialMessageView{
				{RawText: "昵称：A", Media: []collectMediaItem{{Type: "photo", FileId: "p"}}},
				{Media: []collectMediaItem{{Type: "photo", FileId: "p2"}, {Type: "video", FileId: "v"}}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := pairCollectMaterialMessages(test.views)
			if len(got) != len(test.want) {
				t.Fatalf("pairs=%+v want=%+v", got, test.want)
			}
			for index := range got {
				if got[index] != test.want[index] {
					t.Fatalf("pairs=%+v want=%+v", got, test.want)
				}
			}
		})
	}
}
