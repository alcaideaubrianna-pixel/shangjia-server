package sys

import (
	"strings"
	"testing"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestParseBotProfilePublishNoticeTarget(t *testing.T) {
	target, ok := parseBotProfilePublishNoticeTarget("bot_publish:20:-100123:88:123456")
	if !ok || target.BotId != 20 || target.ChatId != "-100123" {
		t.Fatalf("unexpected target: %+v ok:%t", target, ok)
	}
	if _, ok = parseBotProfilePublishNoticeTarget("profile:88:123456"); ok {
		t.Fatal("ordinary publish operation must not create a Bot notice")
	}
}

func TestFormatBotProfilePublishNotice(t *testing.T) {
	createdAt := gtime.New("2026-10-22 19:30:15")
	text := formatBotProfilePublishNotice("A00001", "标题<测试>", createdAt, []botProfilePublishNoticeRow{
		{ChannelTitle: "频道一", ChannelUsername: "public_one", TargetChatId: "-100123", TgMessageId: 10, Status: telegramJobStatusSent},
		{ChannelTitle: "频道<二>", TargetChatId: "-100456", TgMessageId: 20, Status: telegramJobStatusSuperseded},
	})
	for _, expected := range []string{"编号：<code>A00001</code>", "标题：<code>标题&lt;测试&gt;</code>", "2026-10-22 19:30:15", `[频道一] <a href="https://t.me/public_one/10"><b>已发布</b></a>`, `[频道&lt;二&gt;] <a href="https://t.me/c/456/20"><b>已跳过</b></a>`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("notice %q does not contain %q", text, expected)
		}
	}
}
