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

func TestHistoricalIntroFeeFormats(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"colon", "介绍费：7888 情诗X", "介绍费 7888 百花阁"},
		{"joined", "介绍费7888情诗w174", "介绍费 7888 百花阁"},
		{"joined_zero_width", "介绍费13888情诗J69\u2063\u200b\ufeff", "介绍费 13888 百花阁"},
		{"joined_multiple_ids", "介绍费13888情诗w150、151", "介绍费 13888 百花阁"},
		{"recommendation", "推荐费︂:7888", "推荐费 7888 百花阁"},
		{"brokerage", "牵线费：7888 燕姐", "牵线费 7888 百花阁"},
		{"service", "中介服务费：7888 来源J69", "中介服务费 7888 百花阁"},
		{"fullwidth", "介绍费：１２８８８ 卡路里", "介绍费 12888 百花阁"},
		{"prefix", "7888介绍费阿泽", "介绍费 7888 百花阁"},
		{"without_amount", "介绍费：给钱就卖", "介绍费 百花阁"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Replace(tc.line, "百花阁"); got != tc.want {
				t.Fatalf("Replace() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHistoricalIntroFeeDeletionFormats(t *testing.T) {
	for _, line := range []string{
		"介绍费：7888 情诗X",
		"介绍费7888情诗w174",
		"介绍费13888情诗w150、151\u2063\u200b\ufeff",
		"中介服务费：7888 来源J69",
	} {
		if got := RemoveLine(line); got != "" {
			t.Fatalf("RemoveLine(%q) = %q", line, got)
		}
	}
}
