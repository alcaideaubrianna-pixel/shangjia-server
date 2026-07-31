package sys

import (
	"strings"
	"testing"
)

func TestMediaSimilarCandidatesKeepOnlyLiveMedia(t *testing.T) {
	items := []mediaSimilarCandidate{
		{MediaId: 11, ProfileId: 101, Distance: 2},
		{MediaId: 12, ProfileId: 102, Distance: 3},
	}
	filtered := filterMediaSimilarCandidates(&mediaSimilarSource{ProfileId: 999}, items)
	if len(filtered) != len(items) {
		t.Fatalf("filterMediaSimilarCandidates() removed valid candidates: got %d, want %d", len(filtered), len(items))
	}
}

func TestMediaSimilarCountSQLRequiresLiveNoteIndex(t *testing.T) {
	query, _, err := mediaPHashLshProfileCountSQL("0123456789abcdef", 8, []mediaPHashBucketScopePart{{TenantId: 1, AccountIds: []int64{2}}}, "image", 99)
	if err != nil {
		t.Fatalf("mediaPHashLshProfileCountSQL() error = %v", err)
	}
	for _, expected := range []string{
		"hg_youban_publish_note_index",
		"i.deleted_at IS NULL",
		"p.status IN (1, 2)",
		"i.status = p.status",
		"i.visibility = p.visibility",
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("count SQL missing valid note condition %q: %s", expected, query)
		}
	}
}

func TestMediaSimilarLiveProfileIndexRequiresValidConsistentStatus(t *testing.T) {
	query := mediaSimilarLiveProfileIndexExistsSQL("m.profile_id", "m.tenant_id", "m.account_id")
	for _, expected := range []string{
		"p.status IN (1, 2)",
		"i.status = p.status",
		"i.visibility = p.visibility",
		"i.deleted_at IS NULL",
		"p.deleted_at IS NULL",
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("live profile SQL missing %q: %s", expected, query)
		}
	}
}
