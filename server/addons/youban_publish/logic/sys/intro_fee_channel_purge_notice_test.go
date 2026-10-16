package sys

import (
	"strings"
	"testing"
)

func TestCollectChannelPurgeCompletedNotifyText(t *testing.T) {
	text := collectChannelPurgeCompletedNotifyText("XX频道", "-100123", "代理<后缀>", 12, 35, 7, 2)
	for _, want := range []string{"代理&lt;后缀&gt;", "已删除代理资料：12 条", "关联已推送消息：35 条", "已处理推送任务：7 条", "2 个任务的 Telegram 消息清理失败"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}

func TestCollectChannelPurgeSuccessNotifyLink(t *testing.T) {
	text := collectChannelPurgeCompletedNotifyText("XX频道", "-100123", "代理后缀", 12, 35, 7, 0, "https://t.me/example")
	for _, want := range []string{"频道代理资料删除成功", "频道链接：https://t.me/example", "状态：删除并下架成功"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	text = collectChannelPurgeCompletedNotifyText("XX频道", "-100123", "代理后缀", 0, 0, 0, 0)
	if strings.Contains(text, "频道链接") {
		t.Fatalf("unexpected empty link: %s", text)
	}
}

func TestCollectChannelPublicLink(t *testing.T) {
	for _, tc := range []struct{ username, invite, want string }{
		{"@example", "https://t.me/+invite", "https://t.me/example"},
		{"", "https://t.me/+invite", "https://t.me/+invite"},
		{"", "", ""},
	} {
		if got := collectChannelPublicLink(tc.username, tc.invite); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
	if got := collectChannelLinkLine("https://t.me/example?a=1&b=2"); !strings.Contains(got, "&amp;") {
		t.Fatalf("unescaped link: %q", got)
	}
}
