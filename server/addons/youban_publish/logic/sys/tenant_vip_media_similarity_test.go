package sys

import "testing"

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
