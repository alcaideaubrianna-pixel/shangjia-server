package sys

import (
	"testing"

	"github.com/go-telegram/bot/models"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
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
	if isTelegramSearchChat(private) || isTelegramSearchChat(channel) {
		t.Fatal("private and channel messages should not use direct group search")
	}
}

func TestExtractProfileNosSupportsImportedIds(t *testing.T) {
	nos := extractProfileNos("FNUR8829266")
	if len(nos) != 1 || nos[0] != "FNUR8829266" {
		t.Fatalf("expected imported profile id, got %#v", nos)
	}
}
