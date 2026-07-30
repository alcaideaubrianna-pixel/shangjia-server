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
	if !strings.Contains(query, "hg_youban_publish_note_index") || !strings.Contains(query, "i.deleted_at IS NULL") {
		t.Fatalf("count SQL does not require an active note index: %s", query)
	}
}
