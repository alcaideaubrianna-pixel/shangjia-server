package introfee

import "testing"

func TestParseAndReplaceJoinedSourceMarker(t *testing.T) {
	match := Parse("介绍费13888情诗w150、151\u2063\u200b")
	if !match.Matched || match.Amount != "13888" {
		t.Fatalf("unexpected match: %+v", match)
	}
	if got := Replace("介绍费13888情诗w150、151\u2063\u200b", "百花阁"); got != "介绍费 13888 百花阁" {
		t.Fatalf("Replace() = %q", got)
	}
}

func TestReplaceWithoutSuffixDeletesLine(t *testing.T) {
	if got := Replace("介绍费7888来源J69", ""); got != "" {
		t.Fatalf("Replace() = %q", got)
	}
}
