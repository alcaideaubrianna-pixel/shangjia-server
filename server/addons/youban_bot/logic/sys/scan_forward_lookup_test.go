package sys

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestForwardedChannelMessageRef(t *testing.T) {
	msg := &models.Message{ForwardOrigin: &models.MessageOrigin{
		Type: models.MessageOriginTypeChannel,
		MessageOriginChannel: &models.MessageOriginChannel{
			Chat: models.Chat{ID: -1003568590290}, MessageID: 293951,
		},
	}}
	chatId, messageId := forwardedChannelMessageRef(msg)
	if chatId != "-1003568590290" || messageId != 293951 {
		t.Fatalf("unexpected forwarded reference: %s/%d", chatId, messageId)
	}
}

func TestForwardedCaptionProfileNo(t *testing.T) {
	tests := map[string]string{
		"🔰编号:AB2629(点击编号自动复制)\n省份:广东": "AB2629",
		"资料编号：eu9820\n城市：上海":          "EU9820",
		"普通转发内容，没有编号":                 "",
	}
	for input, want := range tests {
		if got := forwardedCaptionProfileNo(input); got != want {
			t.Fatalf("forwardedCaptionProfileNo(%q)=%q, want %q", input, got, want)
		}
	}
}
