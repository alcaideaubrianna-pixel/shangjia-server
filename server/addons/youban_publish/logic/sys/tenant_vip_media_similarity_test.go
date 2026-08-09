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

func TestMediaPHashLshCandidateSQLRequiresLiveMediaAndNote(t *testing.T) {
	query, _, err := mediaPHashLshCandidateSQL("0123456789abcdef", 8, []mediaPHashBucketScopePart{{TenantId: 1, AccountIds: []int64{2}}}, nil, "image", 99)
	if err != nil {
		t.Fatalf("mediaPHashLshCandidateSQL() error = %v", err)
	}
	for _, expected := range []string{
		"hg_youban_publish_media m",
		"m.deleted_at IS NULL",
		"hg_youban_publish_note_index",
		"i.deleted_at IS NULL",
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("candidate SQL missing %q: %s", expected, query)
		}
	}
}

func TestMediaSimilarDeduplicateKeepsBestCandidate(t *testing.T) {
	items := []mediaSimilarCandidate{
		{MediaId: 30, ProfileId: 100, Distance: 4},
		{MediaId: 20, ProfileId: 100, Distance: 2},
		{MediaId: 10, ProfileId: 100, Distance: 2},
		{MediaId: 40, ProfileId: 200, Distance: 3},
	}
	result := mediaSimilarDeduplicate(items)
	if len(result) != 2 {
		t.Fatalf("mediaSimilarDeduplicate() length = %d, want 2", len(result))
	}
	byProfile := make(map[int64]mediaSimilarCandidate, len(result))
	for _, item := range result {
		byProfile[item.ProfileId] = item
	}
	if got := byProfile[100]; got.MediaId != 10 || got.Distance != 2 {
		t.Fatalf("profile 100 candidate = %+v, want media 10 distance 2", got)
	}
	if got := byProfile[200]; got.MediaId != 40 || got.Distance != 3 {
		t.Fatalf("profile 200 candidate = %+v, want media 40 distance 3", got)
	}
}

func TestMediaSimilarResultCacheKeyUsesBoundedStalenessVersion(t *testing.T) {
	key := mediaSimilarResultCacheKey(&mediaSimilarScope{CacheKey: "scope-key"}, &mediaSimilarSource{
		Id:             123,
		PerceptualHash: "0123456789abcdef",
		UpdatedAt:      "2026-08-09 20:00:00",
	}, 8)
	if !strings.HasPrefix(key, "youban_publish:media_similar:v8:scope-key:123:8:") {
		t.Fatalf("unexpected cache key: %s", key)
	}
}
