package sys

import (
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestAdminNoteCandidateCursorUsesCandidateSortValue(t *testing.T) {
	candidateTime := time.Date(2026, 10, 20, 12, 30, 0, 123000000, time.UTC)
	for _, sortBy := range []string{"createdAtDesc", "createdAtAsc", "updatedAtDesc", "publishedAtAsc"} {
		t.Run(sortBy, func(t *testing.T) {
			raw := encodeAdminNoteCandidateCursor(618, gtime.NewFromTime(candidateTime), sortBy)
			cursor, err := decodeAdminNoteCursor(raw)
			if err != nil || cursor == nil {
				t.Fatalf("decode cursor: %v", err)
			}
			if cursor.IndexId != 618 || cursor.SortBy != sortBy || cursor.SortAt != candidateTime.Format(time.RFC3339Nano) {
				t.Fatalf("unexpected cursor: %+v", cursor)
			}
		})
	}
	if got := encodeAdminNoteCandidateCursor(618, nil, "createdAtDesc"); got != "" {
		t.Fatalf("nil sort value must not produce a cursor: %q", got)
	}
}
