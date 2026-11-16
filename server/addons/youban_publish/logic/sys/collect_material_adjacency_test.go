package sys

import (
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	"testing"
	"time"
)

func TestMaterialExplicitParentCannotCrossNewDisplay(t *testing.T) {
	views := []collectMaterialMessageView{
		{EventID: 1, RawText: "这是一份展示资料，文案长度明确超过二十个字符作为展示", Media: []collectMediaItem{{Type: "photo"}}},
		{EventID: 2, RawText: "这是另一份展示资料，文案长度明确超过二十个字符", Media: []collectMediaItem{{Type: "photo"}}},
		{EventID: 3, ParentEventID: 1, Media: []collectMediaItem{{Type: "video"}}},
	}
	if pairs := pairCollectMaterialMessages(views); len(pairs) != 1 || pairs[0] != (collectMaterialPair{DisplayIndex: 1, VerifyIndex: 2}) {
		t.Fatalf("stale parent must yield nearest legal display: %+v", pairs)
	}
}
func TestMaterialExplicitParentConsumesOnlyOneGroup(t *testing.T) {
	views := []collectMaterialMessageView{
		{EventID: 1, RawText: "这是一份展示资料，文案长度明确超过二十个字符作为展示", Media: []collectMediaItem{{Type: "photo"}}},
		{EventID: 2, ParentEventID: 1, Media: []collectMediaItem{{Type: "video"}}},
		{EventID: 3, ParentEventID: 1, Media: []collectMediaItem{{Type: "video"}}},
	}
	if pairs := pairCollectMaterialMessages(views); len(pairs) != 1 || pairs[0].VerifyIndex != 1 {
		t.Fatalf("second verify group attached: %+v", pairs)
	}
}
func TestStoredDisplayStopsAtPreviousVerification(t *testing.T) {
	rows := []gdb.Record{
		{"id": gvar.New(2), "source_grouped_id": gvar.New("old")},
		{"id": gvar.New(1), "raw_text": gvar.New("这是一份展示资料，文案长度明确超过二十个字符作为展示")},
	}
	media := map[int64][]collectMediaItem{2: {{Type: "video"}}, 1: {{Type: "photo"}}}
	verify := gdb.Record{"source_grouped_id": gvar.New("new")}
	if row := collectStoredDisplayFromRows(rows, media, verify); !row.IsEmpty() {
		t.Fatal("crossed previous verification group")
	}
	verify["source_grouped_id"] = gvar.New("old")
	if row := collectStoredDisplayFromRows(rows, media, verify); row["id"].Int64() != 1 {
		t.Fatal("same album continuation lost")
	}
}

func TestLookbackDecidesBeforeTruncatedTail(t *testing.T) {
	rows := []gdb.Record{{"id": gvar.New(200), "raw_text": gvar.New("这是一份展示资料，文案长度明确超过二十个字符作为展示")}}
	media := map[int64][]collectMediaItem{200: {{Type: "photo"}}}
	for i := int64(199); i >= 101; i-- {
		rows = append(rows, gdb.Record{"id": gvar.New(i)})
	}
	row, decided := collectStoredDisplayDecision(rows, media, gdb.Record{})
	if !decided || row["id"].Int64() != 200 {
		t.Fatal("nearest display should decide even if more rows exist")
	}
}
func TestLookbackAllSkippablePageAdvancesKeyset(t *testing.T) {
	rows := []gdb.Record{}
	for i := int64(200); i >= 101; i-- {
		rows = append(rows, gdb.Record{"id": gvar.New(i), "source_message_id": gvar.New(300)})
	}
	if _, decided := collectStoredDisplayDecision(rows, map[int64][]collectMediaItem{}, gdb.Record{}); decided {
		t.Fatal("incomplete skippable page must continue")
	}
	cursor := collectMaterialAdvanceLookback(rows)
	if cursor.MessageID != 300 || cursor.EventID != 101 {
		t.Fatalf("cursor must advance by message and id tie breaker: %+v", cursor)
	}
}

func TestFindDisplayRepairsStaleParentToNearestLegalDisplay(t *testing.T) {
	rows := []gdb.Record{
		{"id": gvar.New(1)},
		{"id": gvar.New(2)},
		{"id": gvar.New(3), "material_parent_event_id": gvar.New(1)},
	}
	views := []collectMaterialMessageView{
		{EventID: 1, RawText: "这是一份展示资料，文案长度明确超过二十个字符作为展示", Media: []collectMediaItem{{Type: "photo"}}},
		{EventID: 2, RawText: "这是另一份展示资料，文案长度明确超过二十个字符", Media: []collectMediaItem{{Type: "photo"}}},
		{EventID: 3, ParentEventID: 1, Media: []collectMediaItem{{Type: "video"}}},
	}
	if index := (&sSysPublish{}).findCollectDisplayEvent(rows, views, 2); index != 1 {
		t.Fatalf("nearest display must replace stale parent in candidate selection: %d", index)
	}
}

func TestMaterialSameGroupMultipleEventsUsesSingleLogicalGroup(t *testing.T) {
	views := []collectMaterialMessageView{
		{EventID: 1, RawText: "这是一份展示资料，文案长度明确超过二十个字符作为展示", Media: []collectMediaItem{{Type: "photo"}}},
		{EventID: 2, ParentEventID: 1, GroupedID: "album", Media: []collectMediaItem{{Type: "video"}}},
		{EventID: 3, ParentEventID: 1, GroupedID: "album", Media: []collectMediaItem{{Type: "video"}}},
		{EventID: 4, GroupedID: "other", Media: []collectMediaItem{{Type: "video"}}},
		{EventID: 5, GroupedID: "album", Media: []collectMediaItem{{Type: "video"}}},
	}
	pairs := pairCollectMaterialMessages(views)
	if len(pairs) != 2 || pairs[0].VerifyIndex != 1 || pairs[1].VerifyIndex != 2 {
		t.Fatalf("same group must allow members but stop at another group: %+v", pairs)
	}
}

func TestLookbackCursorIncludesInsertedEventHighWater(t *testing.T) {
	rows := []gdb.Record{
		{"id": gvar.New(900), "source_message_id": gvar.New(300)},
		{"id": gvar.New(200), "source_message_id": gvar.New(290)},
	}
	cursor := collectMaterialAdvanceLookback(rows)
	if cursor.MessageID != 290 || cursor.EventID != 200 || cursor.HighWaterID != 900 {
		t.Fatalf("continuation must track new inserts separately from descending keyset: %+v", cursor)
	}
}

func TestLookbackPageTwoParentSurvivesCanonicalRecheck(t *testing.T) {
	// The first all-noise page advances to message 101. Finding a display on
	// page two must preserve that cursor until the transaction commits; otherwise
	// canonical validation starts from page one and can never finish binding.
	firstPage := []gdb.Record{}
	for id := int64(200); id >= 101; id-- {
		firstPage = append(firstPage, gdb.Record{"id": gvar.New(id), "source_message_id": gvar.New(id)})
	}
	cursor := collectMaterialAdvanceLookback(firstPage)
	secondPage := []gdb.Record{{"id": gvar.New(100), "source_message_id": gvar.New(100), "raw_text": gvar.New("这是一份展示资料，文案长度明确超过二十个字符作为展示")}}
	media := map[int64][]collectMediaItem{100: {{Type: "photo"}}}
	for attempt := 0; attempt < 2; attempt++ {
		parent, decided := collectStoredDisplayDecision(secondPage, media, gdb.Record{})
		if parent["id"].Int64() != 100 || !decided {
			t.Fatal("second-page display lost")
		}
		if collectMaterialLookbackCanClear(parent, decided, false) {
			t.Fatal("found parent must preserve cursor for canonical validation")
		}
		if cursor.MessageID != 101 || cursor.HighWaterID != 200 {
			t.Fatal("canonical lookup must retain continuation and insert watermark")
		}
	}
	if !collectMaterialLookbackCanClear(nil, true, false) {
		t.Fatal("conclusive negative lookup should clear cursor")
	}
	if collectMaterialLookbackCanClear(nil, false, true) {
		t.Fatal("truncated unresolved lookup must retain cursor")
	}
}

func TestLookbackFutureGenerationPreservesProgress(t *testing.T) {
	cursor := collectMaterialLookbackCursor{MessageID: 101, EventID: 101, HighWaterID: 200, Generation: "before"}
	for _, generation := range []string{"future1", "future2", "future3"} {
		cursor = collectMaterialResumeLookback(cursor, generation)
		if cursor.MessageID != 101 || cursor.EventID != 101 || cursor.HighWaterID != 200 || cursor.Generation != generation {
			t.Fatalf("future ingress discarded progress: %+v", cursor)
		}
	}
}

func TestFindDisplayCannotFallbackAfterGroupConsumed(t *testing.T) {
	rows := []gdb.Record{{"id": gvar.New(1)}, {"id": gvar.New(2)}, {"id": gvar.New(3), "material_parent_event_id": gvar.New(1)}}
	views := []collectMaterialMessageView{
		{EventID: 1, RawText: "这是一份展示资料，文案长度明确超过二十个字符作为展示", Media: []collectMediaItem{{Type: "photo"}}},
		{EventID: 2, Media: []collectMediaItem{{Type: "video"}}},
		{EventID: 3, ParentEventID: 1, Media: []collectMediaItem{{Type: "video"}}},
	}
	if index := (&sSysPublish{}).findCollectDisplayEvent(rows, views, 2); index != -1 {
		t.Fatalf("stored parent bypassed single-group validation: %d", index)
	}
}

func TestVerifySnapshotRejectsConcurrentEdits(t *testing.T) {
	snapshot := gdb.Record{"id": gvar.New(3), "raw_text": gvar.New("编号1"), "updated_at": gvar.New("2026-11-17 01:00:00"), "source_grouped_id": gvar.New("group1"), "source_message_id": gvar.New(200), "tenant_id": gvar.New(1)}
	if !collectMaterialVerifySnapshotMatches(snapshot, snapshot) {
		t.Fatal("unchanged snapshot rejected")
	}
	for _, field := range []string{"raw_text", "media_json", "media_count", "updated_at", "source_grouped_id", "source_message_id", "tenant_id", "material_parent_event_id"} {
		current := gdb.Record{}
		for k, v := range snapshot {
			current[k] = v
		}
		current[field] = gvar.New("changed")
		if collectMaterialVerifySnapshotMatches(snapshot, current) {
			t.Fatalf("concurrent %s edit accepted", field)
		}
	}
}
func TestOldParentRepairRequiresUncommittedUnsharedMaterial(t *testing.T) {
	parent := gdb.Record{"status": gvar.New("group_collecting")}
	if !collectMaterialOldParentRepairSafe(parent, false, false, false) {
		t.Fatal("uncommitted parent should allow repair")
	}
	for _, flags := range [][3]bool{{true, false, false}, {false, true, false}, {false, false, true}} {
		if collectMaterialOldParentRepairSafe(parent, flags[0], flags[1], flags[2]) {
			t.Fatalf("unsafe old relation accepted: %v", flags)
		}
	}
	for _, status := range []string{"processed", "dispatched", "media_ready", "failed", "ignored"} {
		if collectMaterialOldParentRepairSafe(gdb.Record{"status": gvar.New(status)}, false, false, false) {
			t.Fatalf("unsafe old status %s accepted", status)
		}
	}
	if collectMaterialOldParentRepairSafe(gdb.Record{"status": gvar.New("pending"), "processed_at": gvar.New("2026-11-17 01:00:00")}, false, false, false) {
		t.Fatal("processed timestamp must forbid repair")
	}
}

func TestVerifySnapshotPreservesTimestampMicroseconds(t *testing.T) {
	at := time.Date(2026, time.November, 17, 1, 0, 0, 123456000, time.UTC)
	before := gdb.Record{"id": gvar.New(3), "updated_at": gvar.New(gtime.NewFromTime(at))}
	same := gdb.Record{"id": gvar.New(3), "updated_at": gvar.New(gtime.NewFromTime(at.In(time.FixedZone("UTC+8", 8*60*60))))}
	if !collectMaterialVerifySnapshotMatches(before, same) {
		t.Fatal("same instant in another zone must match")
	}
	changed := gdb.Record{"id": gvar.New(3), "updated_at": gvar.New(gtime.NewFromTime(at.Add(time.Microsecond)))}
	if collectMaterialVerifySnapshotMatches(before, changed) {
		t.Fatal("same-second microsecond edit must invalidate snapshot")
	}
}
