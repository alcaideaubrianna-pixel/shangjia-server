package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestFindCollectVerifyEventUsesOneFollowingVideoGroup(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-4 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(1), "source_message_id": gvar.New(100), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A100"), "media_json": gvar.New(`[{"type":"photo","fileId":"photo-100"}]`), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(2), "source_message_id": gvar.New(101), "material_role": gvar.New("pending"), "raw_text": gvar.New("频道通知"), "media_json": gvar.New(`[]`), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(3), "source_message_id": gvar.New(102), "material_role": gvar.New("pending"), "raw_text": gvar.New(""), "media_json": gvar.New(`[{"type":"video","fileId":"video-102"}]`), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(4), "source_message_id": gvar.New(103), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A101"), "media_json": gvar.New(`[{"type":"photo","fileId":"photo-103"}]`), "received_at": gvar.New(receivedAt)},
	}
	if got := (&sSysPublish{}).findCollectVerifyEvent(rows, 0); got != 2 {
		t.Fatalf("verify event index=%d, want 2", got)
	}
}

func TestFindCollectVerifyEventDoesNotCrossNextDisplayGroup(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-4 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(1), "source_message_id": gvar.New(100), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A100"), "media_json": gvar.New(`[{"type":"photo","fileId":"photo-100"}]`), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(2), "source_message_id": gvar.New(101), "material_role": gvar.New("pending"), "raw_text": gvar.New("昵称：A101"), "media_json": gvar.New(`[{"type":"photo","fileId":"photo-101"}]`), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(3), "source_message_id": gvar.New(102), "material_role": gvar.New("pending"), "raw_text": gvar.New(""), "media_json": gvar.New(`[{"type":"video","fileId":"video-102"}]`), "received_at": gvar.New(receivedAt)},
	}
	if got := (&sSysPublish{}).findCollectVerifyEvent(rows, 0); got != -1 {
		t.Fatalf("verify event index=%d, want no cross-group match", got)
	}
}

func TestFindCollectDisplayEventRepairsAlreadyPairedVerify(t *testing.T) {
	receivedAt := gtime.NewFromTime(time.Now().Add(-4 * time.Minute))
	rows := []gdb.Record{
		{"id": gvar.New(10), "source_message_id": gvar.New(200), "material_role": gvar.New("display"), "material_parent_event_id": gvar.New(0), "raw_text": gvar.New("昵称：A200"), "media_json": gvar.New(`[{"type":"photo","fileId":"photo-200"}]`), "received_at": gvar.New(receivedAt)},
		{"id": gvar.New(11), "source_message_id": gvar.New(201), "material_role": gvar.New("verify"), "material_parent_event_id": gvar.New(10), "status": gvar.New("ignored"), "error_message": gvar.New(collectMaterialVerifyUnmatchedMessage), "raw_text": gvar.New(""), "media_json": gvar.New(`[{"type":"video","fileId":"video-201"}]`), "received_at": gvar.New(receivedAt)},
	}
	if got := (&sSysPublish{}).findCollectDisplayEvent(rows, 1); got != 0 {
		t.Fatalf("display event index=%d, want 0", got)
	}
	if !collectMaterialEventNeedsPairRepair(rows[1]) {
		t.Fatal("already ignored unmatched verify event should be repairable")
	}
}

func TestCollectMaterialEventOlderThanUsesDatabaseWallClock(t *testing.T) {
	databaseTime := time.Now().Add(-4 * time.Minute).UTC()
	event := gdb.Record{"received_at": gvar.New(databaseTime)}
	if !collectMaterialEventOlderThan(event, 3*time.Minute) {
		t.Fatalf("database wall-clock event should be considered older than grouping window")
	}
}
