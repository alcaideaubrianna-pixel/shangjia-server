package sys

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
)

func TestQuickPushNavigationText(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "empty text stays in flow", text: "", want: false},
		{name: "command exits flow", text: "/start", want: true},
		{name: "menu label exits flow", text: "联系客服", want: true},
		{name: "quick push label exits flow", text: "快速推送", want: true},
		{name: "ordinary content stays in flow", text: "朴朴芙蓉B3054", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := quickPushNavigationText(test.text); got != test.want {
				t.Fatalf("quickPushNavigationText(%q) = %v, want %v", test.text, got, test.want)
			}
		})
	}
}

func TestQuickPushPlanDisplayName(t *testing.T) {
	if got := quickPushPlanDisplayName(&publishsysin.QuickPushPlanModel{Name: "群聊推送", SerialNo: "QP000001"}); got != "群聊推送" {
		t.Fatalf("expected plan name, got %q", got)
	}
	if got := quickPushPlanDisplayName(&publishsysin.QuickPushPlanModel{SerialNo: "QP000001"}); got != "QP000001" {
		t.Fatalf("expected serial fallback, got %q", got)
	}
}

func TestQuickPushPlanKeyboardIncludesTemplateSave(t *testing.T) {
	session := &quickPushSession{SessionId: "session", SelectedPlanIds: []int64{1}}
	plans := []*publishsysin.QuickPushPlanModel{{Id: 1, Name: "群聊推送"}}
	keyboard := quickPushPlanKeyboard(session, plans)
	if !quickPushKeyboardHasButton(keyboard, "保存模板", quickPushCallbackData("save", session.SessionId, 0)) {
		t.Fatal("expected save template button")
	}

	session.SavedTemplateId = 100
	keyboard = quickPushPlanKeyboard(session, plans)
	if !quickPushKeyboardHasButton(keyboard, "✅ 模板已保存", quickPushCallbackData("save", session.SessionId, 0)) {
		t.Fatal("expected saved template button")
	}
}

func TestQuickPushMediaUploadContext(t *testing.T) {
	ctx := quickPushMediaUploadContext(context.Background(), 123)
	if got := contexts.GetModule(ctx); got != consts.AppApi {
		t.Fatalf("expected api module, got %q", got)
	}
	if got := contexts.GetUserId(ctx); got != 123 {
		t.Fatalf("expected operator account id, got %d", got)
	}
}

func quickPushKeyboardHasButton(keyboard *models.InlineKeyboardMarkup, text string, callbackData string) bool {
	if keyboard == nil {
		return false
	}
	for _, row := range keyboard.InlineKeyboard {
		for _, button := range row {
			if button.Text == text && button.CallbackData == callbackData {
				return true
			}
		}
	}
	return false
}

func TestQuickPushTelegramMessageTextKeepsCustomEmoji(t *testing.T) {
	msg := &models.Message{
		Text: "A😀X后缀",
		Entities: []models.MessageEntity{{
			Type:          models.MessageEntityTypeCustomEmoji,
			Offset:        3,
			Length:        1,
			CustomEmojiID: "5368324170671202286",
		}},
	}
	want := `A😀<tg-emoji emoji-id="5368324170671202286">X</tg-emoji>后缀`
	if got := quickPushTelegramMessageText(msg, ""); got != want {
		t.Fatalf("unexpected custom emoji html: %q", got)
	}
}

func TestQuickPushTelegramMessageCaptionKeepsCustomEmoji(t *testing.T) {
	msg := &models.Message{
		Caption: "图文🙂",
		CaptionEntities: []models.MessageEntity{{
			Type:          models.MessageEntityTypeCustomEmoji,
			Offset:        2,
			Length:        2,
			CustomEmojiID: "emoji-id",
		}},
	}
	want := `图文<tg-emoji emoji-id="emoji-id">🙂</tg-emoji>`
	if got := quickPushTelegramMessageText(msg, ""); got != want {
		t.Fatalf("unexpected custom emoji caption html: %q", got)
	}
}

func TestQuickPushTelegramMessageTextKeepsNestedFormatting(t *testing.T) {
	msg := &models.Message{
		Text: "粗体链接🙂",
		Entities: []models.MessageEntity{
			{Type: models.MessageEntityTypeBold, Offset: 0, Length: 4},
			{Type: models.MessageEntityTypeTextLink, Offset: 2, Length: 2, URL: "https://example.com?a=1&b=2"},
			{Type: models.MessageEntityTypeCustomEmoji, Offset: 4, Length: 2, CustomEmojiID: "emoji-id"},
		},
	}
	want := `<b>粗体<a href="https://example.com?a=1&amp;b=2">链接</a></b><tg-emoji emoji-id="emoji-id">🙂</tg-emoji>`
	if got := quickPushTelegramMessageText(msg, ""); got != want {
		t.Fatalf("unexpected nested formatting html: %q", got)
	}
}

func TestTelegramEntityHTMLTagsSupportsFormatting(t *testing.T) {
	tests := []struct {
		entity models.MessageEntity
		open   string
		close  string
	}{
		{entity: models.MessageEntity{Type: models.MessageEntityTypeItalic}, open: "<i>", close: "</i>"},
		{entity: models.MessageEntity{Type: models.MessageEntityTypeUnderline}, open: "<u>", close: "</u>"},
		{entity: models.MessageEntity{Type: models.MessageEntityTypeStrikethrough}, open: "<s>", close: "</s>"},
		{entity: models.MessageEntity{Type: models.MessageEntityTypeSpoiler}, open: "<tg-spoiler>", close: "</tg-spoiler>"},
		{entity: models.MessageEntity{Type: models.MessageEntityTypeExpandableBlockquote}, open: "<blockquote expandable>", close: "</blockquote>"},
		{entity: models.MessageEntity{Type: models.MessageEntityTypeCode}, open: "<code>", close: "</code>"},
		{entity: models.MessageEntity{Type: models.MessageEntityTypePre, Language: "go"}, open: `<pre><code class="language-go">`, close: "</code></pre>"},
	}
	for _, test := range tests {
		open, close, ok := telegramEntityHTMLTags(test.entity)
		if !ok || open != test.open || close != test.close {
			t.Fatalf("entity %s tags = %q %q %v", test.entity.Type, open, close, ok)
		}
	}
}

func TestTelegramHTTPClientAllowsDirectConnection(t *testing.T) {
	client, err := telegramHTTPClient("")
	if err != nil {
		t.Fatalf("direct client should not fail: %v", err)
	}
	if client == nil || client.Transport == nil {
		t.Fatal("direct client should use a transport")
	}
}

func TestTelegramChatRouting(t *testing.T) {
	private := &models.Message{Chat: models.Chat{Type: "private"}}
	group := &models.Message{Chat: models.Chat{Type: "group"}}
	channel := &models.Message{Chat: models.Chat{Type: "channel"}}
	if !isTelegramPrivateChat(private) || isTelegramPrivateChat(group) {
		t.Fatal("scan media should only route private chats")
	}
	if !isTelegramSearchChat(group) || !isTelegramSearchChat(&models.Message{Chat: models.Chat{Type: "supergroup"}}) {
		t.Fatal("group messages should route to note search")
	}
	if isTelegramSearchChat(private) || !isTelegramSearchChat(channel) {
		t.Fatal("private messages should not search, channel messages should support note search")
	}
}

func TestExtractProfileNosSupportsImportedIds(t *testing.T) {
	nos := extractProfileNos("FNUR8829266")
	if len(nos) != 1 || nos[0] != "FNUR8829266" {
		t.Fatalf("expected imported profile id, got %#v", nos)
	}
}
