package sys

import (
	"testing"
)

func TestNormalizeHomeProfileFeedRecommended(t *testing.T) {
	if got := normalizeHomeProfileFeed("recommended", ""); got != homeProfileFeedRecommended {
		t.Fatalf("normalizeHomeProfileFeed() = %q, want %q", got, homeProfileFeedRecommended)
	}
}

func TestProfileRankOrderExpression(t *testing.T) {
	want := "CASE p.id WHEN 42 THEN 0 WHEN 7 THEN 1 ELSE 2 END"
	if got := profileRankOrderExpression("p.id", []int64{42, 7}); got != want {
		t.Fatalf("profileRankOrderExpression() = %q, want %q", got, want)
	}
	if got := profileRankOrderExpression("p.id", nil); got != "0" {
		t.Fatalf("empty profileRankOrderExpression() = %q, want 0", got)
	}
}
