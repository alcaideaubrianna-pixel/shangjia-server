package sys

import (
	"testing"

	"hotgo/addons/youban_publish/model"
)

func TestMatchedAutoDeleteRule(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		rules []string
		want  string
	}{
		{
			name:  "single number line",
			text:  "编号: 431177",
			rules: []string{autoDeleteRuleSingleNumberLine},
			want:  autoDeleteRuleSingleNumberLine,
		},
		{
			name:  "single number line with content",
			text:  "酒鬼全国飞包频道\n编号: 431177",
			rules: []string{autoDeleteRuleSingleNumberLine},
			want:  "",
		},
		{
			name:  "custom single line rule",
			text:  "仅测试",
			rules: []string{`single:^仅测试$`},
			want:  `single:^仅测试$`,
		},
		{
			name:  "multi line text rule",
			text:  "编号: 431177\n视频",
			rules: []string{`text:^编号`},
			want:  `text:^编号`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchedAutoDeleteRule(tt.text, tt.rules)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEnsureAutoDeleteDefaultRules(t *testing.T) {
	conf := &model.AutoDeleteConfig{Rules: []string{}}
	ensureAutoDeleteDefaultRules(conf)
	if len(conf.Rules) != 1 || conf.Rules[0] != autoDeleteRuleSingleNumberLine {
		t.Fatalf("unexpected default rules: %#v", conf.Rules)
	}
	conf.Rules = []string{`single:^仅测试$`}
	ensureAutoDeleteDefaultRules(conf)
	if len(conf.Rules) != 2 || conf.Rules[0] != autoDeleteRuleSingleNumberLine {
		t.Fatalf("unexpected merged rules: %#v", conf.Rules)
	}
}
