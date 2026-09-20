package sys

import (
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestNormalizeAdminNoteSortBy(t *testing.T) {
	for input, want := range map[string]string{"": adminNoteSortCreatedAt, adminNoteSortCreatedAt: adminNoteSortCreatedAt, adminNoteSortPublishedAt: adminNoteSortPublishedAt} {
		got, err := normalizeAdminNoteSortBy(input)
		if err != nil || got != want {
			t.Fatalf("normalizeAdminNoteSortBy(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := normalizeAdminNoteSortBy("updatedAt"); err == nil {
		t.Fatal("unsupported sort field must fail")
	}
}

func TestAdminNoteCursorUsesSelectedSort(t *testing.T) {
	createdAt := gtime.NewFromTime(time.Date(2026, 9, 1, 1, 2, 3, 0, time.UTC))
	publishedAt := gtime.NewFromTime(time.Date(2026, 9, 2, 1, 2, 3, 0, time.UTC))
	item := &sysin.ProfileModel{Id: 9, NoteIndexId: 19, CreatedAt: createdAt, PublishedAt: publishedAt}
	for _, sortBy := range []string{adminNoteSortCreatedAt, adminNoteSortPublishedAt} {
		cursor, err := decodeAdminNoteCursor(encodeAdminNoteCursor(item, sortBy))
		if err != nil || cursor.SortBy != sortBy || cursor.IndexId != 19 {
			t.Fatalf("cursor for %s = %+v, %v", sortBy, cursor, err)
		}
	}
}

func TestAdminNotePublishedSortPlacesUnpublishedLast(t *testing.T) {
	expression := adminNoteSortExpression(adminNoteSortPublishedAt)
	if !strings.Contains(expression, "COALESCE(i.published_at") || !strings.Contains(expression, "1970-01-01") {
		t.Fatalf("published sort expression = %q", expression)
	}
}
