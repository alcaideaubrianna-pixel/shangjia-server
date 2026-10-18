package sys

import (
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	"strings"
	"testing"
)

func TestCollectChannelDeleteNotice(t *testing.T) {
	item := &publishsysin.IntroFeeChannelRuleItem{ChannelTitle: "XX<频道>", ChannelId: "-100123", Suffix: "北京&代理", ChannelLink: "https://t.me/example"}
	for _, tc := range []struct {
		name   string
		result publishsysin.IntroFeeChannelPurgeModel
		want   string
		absent string
	}{
		{"queued", publishsysin.IntroFeeChannelPurgeModel{ProfileCount: 12, MessageCount: 35, Queued: 1}, "完成后会再次通知", "无关联代理资料"},
		{"empty", publishsysin.IntroFeeChannelPurgeModel{}, "无关联代理资料或已推送消息需要删除", "完成后会再次通知"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := collectChannelDeleteNotice(item, &tc.result)
			for _, want := range []string{"XX&lt;频道&gt;", "北京&amp;代理", "-100123", "频道链接：https://t.me/example", "操作原因：管理员主动删除", tc.want} {
				if !strings.Contains(text, want) {
					t.Fatalf("missing %q in %q", want, text)
				}
			}
			if strings.Contains(text, tc.absent) {
				t.Fatalf("unexpected %q in %q", tc.absent, text)
			}
			if tc.result.Queued == 1 && (!strings.Contains(text, "12 条") || !strings.Contains(text, "35 条")) {
				t.Fatalf("missing counts: %s", text)
			}
		})
	}
}

func TestCollectManageChannelLinkLine(t *testing.T) {
	if got := collectManageChannelLinkLine(""); got != "" {
		t.Fatalf("empty link must be omitted: %q", got)
	}
	if got := collectManageChannelLinkLine("https://t.me/example?a=1&b=2"); !strings.Contains(got, "&amp;") {
		t.Fatalf("unescaped link: %q", got)
	}
}
