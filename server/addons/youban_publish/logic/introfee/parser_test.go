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
		{"recommendation", "推荐费︂:7888", "介绍费 7888 百花阁"},
		{"brokerage", "牵线费：7888 燕姐", "介绍费 7888 百花阁"},
		{"service", "中介服务费：7888 来源J69", "介绍费 7888 百花阁"},
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

func TestProcessHistoricalDeletionRules(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"tail", "正文内容\n介绍费7888情诗J69\n联系方式", "正文内容"},
		{"short_previous", "正文内容\n橙子\n介绍费：7888\n尾部", "正文内容"},
		{"five_visible_previous", "正文内容\n天空之城店\n介绍费：7888\n尾部", "正文内容"},
		{"long_previous", "正文内容\n正常标题内容\n介绍费：7888\n尾部", "正文内容\n正常标题内容"},
		{"blank_before_fee", "正文内容\n橙子\n\n介绍费：7888\n尾部", "正文内容"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Process(tc.text, Policy{RemoveTail: true}); got != tc.want {
				t.Fatalf("Process() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProcessHistoricalSuffixRules(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"joined", "正文\n介绍费7888情诗J69\n联系方式", "正文\n介绍费 7888 百花阁\n联系方式"},
		{"multiple_rows", "推荐费：7888 情诗X\n介绍费:7889", "介绍费 7888 百花阁\n介绍费 7889 百花阁"},
		{"inline", "正文 自我介绍 介绍费7888 阿泽", "正文 自我介绍 介绍费 7888 百花阁"},
		{"fullwidth", "正文\n介绍费：１２８８８ 卡路里", "正文\n介绍费 12888 百花阁"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Process(tc.text, Policy{Suffix: "百花阁"}); got != tc.want {
				t.Fatalf("Process() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHistoricalIntroFeePolicyBoundaries(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		policy Policy
		want   string
	}{
		{
			name:   "disabled keeps original",
			text:   "正文\n介绍费 7888 情诗X\n联系方式",
			policy: Policy{},
			want:   "正文\n介绍费 7888 情诗X\n联系方式",
		},
		{
			name:   "footer remains after suffix replacement",
			text:   "正文\n介绍费：7888 情诗X\n联系方式",
			policy: Policy{Suffix: "百花阁"},
			want:   "正文\n介绍费 7888 百花阁\n联系方式",
		},
		{
			name:   "fee tail removes all later text",
			text:   "正文\n介绍费：7888\n联系方式\n更多资料",
			policy: Policy{RemoveTail: true},
			want:   "正文",
		},
		{
			name:   "zero width keyword markers are normalized",
			text:   "正文\n牵\u200b线\ufe0f费︂：7888\n联系方式",
			policy: Policy{Suffix: "百花阁"},
			want:   "正文\n介绍费 7888 百花阁\n联系方式",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Process(tc.text, tc.policy); got != tc.want {
				t.Fatalf("Process() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseFieldsHistoricalAmounts(t *testing.T) {
	cases := []struct {
		name string
		text string
		want Fields
	}{
		{"domestic", "介绍费：国内7888，国外12888", Fields{FirstAmount: "7888", ForeignAmount: "12888"}},
		{"prefix source", "阿泽介绍费7888", Fields{FirstAmount: "7888", FirstSuffix: "阿泽"}},
		{"joined source", "介绍费7888情诗w174", Fields{FirstAmount: "7888"}},
		{"profile numbers ignored", "身高168\n介绍费7888\n年龄22", Fields{FirstAmount: "7888"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseFields(tc.text); got != tc.want {
				t.Fatalf("ParseFields() = %#v, want %#v", got, tc.want)
			}
		})
	}
}
