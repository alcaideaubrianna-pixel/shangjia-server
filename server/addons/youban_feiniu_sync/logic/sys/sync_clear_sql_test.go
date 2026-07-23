package sys

import (
	"strings"
	"testing"
)

func TestUniqueInt64s(t *testing.T) {
	got := uniqueInt64s([]int64{0, 3, 2, 3, -1, 2, 5})
	want := []int64{3, 2, 5}
	if len(got) != len(want) {
		t.Fatalf("unexpected length: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected value at %d: got %d want %d", i, got[i], want[i])
		}
	}
}

func TestChannelClearSQLUsesSubquery(t *testing.T) {
	sqls := []string{
		channelClearTaskMediaSQL(),
		channelClearTaskSQL(),
		channelClearProfileMediaSQL("hg_content_media", "deleted_at"),
		channelClearProfileSQL("hg_content_profile", "deleted_at", "updated_at"),
		channelClearAccountSQL(),
	}
	for _, sql := range sqls {
		if !strings.Contains(sql, "SELECT DISTINCT") {
			t.Fatalf("expected subquery sql, got: %s", sql)
		}
		if !strings.Contains(sql, `WHERE "config_id" = ?`) {
			t.Fatalf("expected config scoped sql, got: %s", sql)
		}
	}
}
