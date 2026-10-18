package sysin

import "testing"

func TestMatchCollectKeyword(t *testing.T) {
	for _, tc := range []struct {
		name, text, keyword string
		want                bool
	}{
		{"plain substring", "HELLO 世界", "hello", true},
		{"plain metacharacters", "上海", "上海|北京", false},
		{"regex alternatives", "今日北京资料", "regex:上海|北京", true},
		{"regex anchors", "编号：123", `regex:^编号：\d+$`, true},
		{"regex anchor miss", "今日编号：123", `regex:^编号：\d+$`, false},
		{"case insensitive", "ABC", "regex:^abc$", true},
		{"explicit case sensitive", "ABC", "regex:(?-i:^abc$)", false},
		{"literal reserved prefix", "regex:abc", "text:regex:abc", true},
		{"literal text prefix", "text:abc", "text:text:abc", true},
		{"empty media cannot match", "", "regex:^$", false},
		{"invalid regex fails closed", "abc", "regex:[", false},
		{"uppercase escape", "abc", `regex:^\D+$`, true},
		{"lowercase escape", "abc", `regex:^\d+$`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchCollectKeyword(tc.text, tc.keyword); got != tc.want {
				t.Fatalf("match=%t want %t", got, tc.want)
			}
		})
	}
}

func TestNormalizeCollectKeywords(t *testing.T) {
	values, err := NormalizeCollectKeywords([]string{" hello ", "HELLO", `regex:\d`, `regex:\D`, "regex:abc", "regex:ABC", ""})
	if err != nil || len(values) != 5 {
		t.Fatalf("values=%q err=%v", values, err)
	}
	for _, invalid := range []string{"regex:", "text:", "regex:[", "regex:(?=abc)", `regex:(a)\1`} {
		if _, err := NormalizeCollectKeywords([]string{invalid}); err == nil {
			t.Errorf("accepted invalid keyword %q", invalid)
		}
	}
}

func TestCollectRuleFilterRejectsInvalidRegex(t *testing.T) {
	in := &CollectRuleSaveInp{Name: "规则", TargetChannelIds: []int64{1}, Keywords: []string{"regex:["}}
	if err := in.Filter(t.Context()); err == nil {
		t.Fatal("saving invalid regex must fail")
	}
}
