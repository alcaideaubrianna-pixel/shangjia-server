package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestMergeSourceNoteRowsDeduplicatesByNoteID(t *testing.T) {
	row := func(id int64) gdb.Record {
		return gdb.Record{"note_id": gvar.New(id)}
	}
	rows := mergeSourceNoteRows([]gdb.Record{row(3), row(1)}, []gdb.Record{row(1), row(2)})
	if len(rows) != 3 {
		t.Fatalf("mergeSourceNoteRows() returned %d rows, want 3", len(rows))
	}
	if got := rows[0]["note_id"].Int64(); got != 3 {
		t.Fatalf("first row id = %d, want 3", got)
	}
	if got := rows[1]["note_id"].Int64(); got != 1 {
		t.Fatalf("second row id = %d, want 1", got)
	}
	if got := rows[2]["note_id"].Int64(); got != 2 {
		t.Fatalf("third row id = %d, want 2", got)
	}
}

func TestLatestSourceUpdateCursorUsesTimeAndNoteID(t *testing.T) {
	base := gtime.NewFromTime(time.Date(2026, 7, 26, 10, 0, 0, 0, time.Local))
	later := base.Add(time.Minute)
	rows := []gdb.Record{
		{"note_id": gvar.New(8), "update_time": gvar.New(base)},
		{"note_id": gvar.New(2), "update_time": gvar.New(later)},
		{"note_id": gvar.New(9), "update_time": gvar.New(later)},
	}
	gotTime, gotNoteID := latestSourceUpdateCursor(rows, base, 3)
	if gotTime == nil || !gotTime.Time.Equal(later.Time) {
		t.Fatalf("latest cursor time = %v, want %v", gotTime, later)
	}
	if gotNoteID != 9 {
		t.Fatalf("latest cursor note id = %d, want 9", gotNoteID)
	}
}

func TestIsSourceMediaPending(t *testing.T) {
	record := func(ingest string, expected, actual, ready int) gdb.Record {
		return gdb.Record{
			"ingest_status":            gvar.New(ingest),
			"image_count":              gvar.New(expected),
			"video_count":              gvar.New(0),
			"source_media_count":       gvar.New(actual),
			"source_ready_media_count": gvar.New(ready),
		}
	}
	tests := []struct {
		name string
		row  gdb.Record
		want bool
	}{
		{name: "ingest pending", row: record("pending_review", 1, 1, 1), want: true},
		{name: "media blocks missing", row: record("done", 2, 1, 1), want: true},
		{name: "cos backfill pending", row: record("done", 2, 2, 1), want: true},
		{name: "media ready", row: record("done", 2, 2, 2), want: false},
		{name: "text only ready", row: record("done", 0, 0, 0), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSourceMediaPending(tt.row); got != tt.want {
				t.Fatalf("isSourceMediaPending() = %t, want %t", got, tt.want)
			}
		})
	}
}
