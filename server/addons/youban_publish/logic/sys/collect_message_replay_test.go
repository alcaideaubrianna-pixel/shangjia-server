package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestProductionLateMediaBoundarySamples(t *testing.T) {
	tests := []struct {
		name            string
		dispatchCreated string
		mediaCreated    []string
		wantLate        int
	}{
		{name: "profile-712355", dispatchCreated: "2026-10-30T11:52:05.724786", mediaCreated: []string{"2026-10-30T11:51:59.514737", "2026-10-30T11:51:59.526597", "2026-10-30T13:18:20.766808"}, wantLate: 1},
		{name: "profile-716593", dispatchCreated: "2026-11-01T19:44:48.994715", mediaCreated: []string{"2026-11-01T19:39:03.952819", "2026-11-01T19:44:12.310683", "2026-11-01T19:53:36.259848", "2026-11-01T19:53:37.284711", "2026-11-01T19:53:39.620195"}, wantLate: 3},
		{name: "profile-716709", dispatchCreated: "2026-11-01T20:52:51.335127", mediaCreated: []string{"2026-11-01T20:52:22.621909", "2026-11-01T20:52:23.847004", "2026-11-01T20:52:23.892394", "2026-11-01T21:00:14.928487", "2026-11-01T21:00:18.551858"}, wantLate: 2},
		{name: "profile-717277", dispatchCreated: "2026-11-02T16:20:34.743632", mediaCreated: []string{"2026-11-02T16:18:02.782936", "2026-11-02T16:19:23.977784", "2026-11-02T19:45:35.169398", "2026-11-02T19:45:38.049686"}, wantLate: 2},
		{name: "profile-717329", dispatchCreated: "2026-11-02T17:34:41.081217", mediaCreated: []string{"2026-11-02T17:22:09.047085", "2026-11-02T17:22:09.987461", "2026-11-02T17:22:15.847163", "2026-11-02T20:17:15.483347"}, wantLate: 1},
		{name: "profile-717421", dispatchCreated: "2026-11-02T18:27:54.892094", mediaCreated: []string{"2026-11-02T18:27:26.412273", "2026-11-02T18:27:41.999786", "2026-11-02T18:28:03.532859", "2026-11-02T18:28:05.760791", "2026-11-02T18:28:21.244642"}, wantLate: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boundary := mustParseProductionFixtureTime(t, tt.dispatchCreated)
			late := 0
			for _, value := range tt.mediaCreated {
				if mustParseProductionFixtureTime(t, value).After(boundary) {
					late++
				}
			}
			if late != tt.wantLate {
				t.Fatalf("late media = %d, want %d", late, tt.wantLate)
			}
		})
	}
}

func mustParseProductionFixtureTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02T15:04:05.999999", value)
	if err != nil {
		t.Fatalf("parse fixture time %q: %v", value, err)
	}
	return parsed
}

func TestDecideCollectExistingEventMerge(t *testing.T) {
	item := collectMediaItem{Type: "image", FileId: "file-1", SourceMediaId: 101, SourceAccessHash: 202}
	key := "image:" + collectMediaSourceKey(item)

	tests := []struct {
		name   string
		stored map[string]gdb.Record
		want   collectExistingEventMergeDecision
	}{
		{name: "new media", stored: map[string]gdb.Record{}, want: collectExistingEventMergeDecision{ShouldMerge: true, HasNewMedia: true}},
		{name: "telegram identity repaired", stored: map[string]gdb.Record{key: {"source_media_id": g.NewVar(int64(100)), "source_access_hash": g.NewVar(int64(201))}}, want: collectExistingEventMergeDecision{ShouldMerge: true}},
		{name: "unchanged", stored: map[string]gdb.Record{key: {"source_media_id": g.NewVar(int64(101)), "source_access_hash": g.NewVar(int64(202))}}, want: collectExistingEventMergeDecision{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := decideCollectExistingEventMerge([]collectMediaItem{item}, tt.stored); got != tt.want {
				t.Fatalf("decision = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestCollectReplayUpdateFieldsResetsTerminalGroupingState(t *testing.T) {
	event := gdb.Record{
		"status":                   gvar.New(sysin.CollectEventStatusIgnored),
		"processed_at":             gvar.New(gtime.Now()),
		"material_role":            gvar.New(collectMaterialRoleDisplay),
		"material_parent_event_id": gvar.New(int64(12)),
		"material_group_status":    gvar.New("paired"),
	}
	update := collectReplayUpdateFields(event, &CollectMessage{SourceGroupedId: "album"}, gtime.Now())

	if got := update["status"]; got != sysin.CollectEventStatusGroupCollect {
		t.Fatalf("status = %v, want %s", got, sysin.CollectEventStatusGroupCollect)
	}
	if got := update["material_role"]; got != collectMaterialRolePending {
		t.Fatalf("material_role = %v, want %s", got, collectMaterialRolePending)
	}
	if got := update["material_parent_event_id"]; got != 0 {
		t.Fatalf("material_parent_event_id = %v, want 0", got)
	}
	if got := update["material_group_status"]; got != "" {
		t.Fatalf("material_group_status = %v, want empty", got)
	}
	if _, ok := update["processed_at"]; !ok || update["processed_at"] != nil {
		t.Fatalf("processed_at = %v, want explicit nil", update["processed_at"])
	}
}

func TestCollectReplayUpdateFieldsKeepsActiveGroupingState(t *testing.T) {
	event := gdb.Record{"status": gvar.New(sysin.CollectEventStatusGroupCollect)}
	update := collectReplayUpdateFields(event, &CollectMessage{SourceGroupedId: "album"}, gtime.Now())

	for _, field := range []string{"processed_at", "material_role", "material_parent_event_id", "material_group_status"} {
		if _, ok := update[field]; ok {
			t.Fatalf("active event update unexpectedly contains %s", field)
		}
	}
}
