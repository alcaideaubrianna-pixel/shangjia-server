package sys

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestReplyKeyboardDeliveryVersion(t *testing.T) {
	keyboard := &models.ReplyKeyboardMarkup{
		Keyboard: [][]models.KeyboardButton{
			{{Text: "开始使用"}, {Text: "立即注册"}},
		},
		IsPersistent:          true,
		ResizeKeyboard:        true,
		InputFieldPlaceholder: "请选择菜单",
	}

	version := replyKeyboardDeliveryVersion(keyboard)
	if version == "" {
		t.Fatal("keyboard version should not be empty")
	}
	if got := replyKeyboardDeliveryVersion(keyboard); got != version {
		t.Fatalf("keyboard version should be deterministic, got %q and %q", version, got)
	}

	changedKeyboard := &models.ReplyKeyboardMarkup{
		Keyboard:              [][]models.KeyboardButton{{{Text: "开始使用"}}},
		IsPersistent:          true,
		ResizeKeyboard:        true,
		InputFieldPlaceholder: "请选择菜单",
	}
	if got := replyKeyboardDeliveryVersion(changedKeyboard); got == version {
		t.Fatal("keyboard content change should produce a new version")
	}
	if got := replyKeyboardDeliveryVersion(&models.ReplyKeyboardRemove{RemoveKeyboard: true}); got == version {
		t.Fatal("keyboard removal should produce a different version")
	}
}

func TestIsReplyKeyboardMarkup(t *testing.T) {
	if !isReplyKeyboardMarkup(&models.ReplyKeyboardMarkup{}) {
		t.Fatal("reply keyboard should be recognized")
	}
	if !isReplyKeyboardMarkup(&models.ReplyKeyboardRemove{RemoveKeyboard: true}) {
		t.Fatal("reply keyboard removal should be recognized")
	}
	if isReplyKeyboardMarkup(&models.InlineKeyboardMarkup{}) {
		t.Fatal("inline keyboard should not be recognized as persistent reply keyboard")
	}
	if isReplyKeyboardMarkup(nil) {
		t.Fatal("nil markup should not be recognized")
	}
}

func TestIsTelegramCommand(t *testing.T) {
	for _, text := range []string{"/start", " /unknown argument", "/menu@sample_bot"} {
		if !isTelegramCommand(text) {
			t.Fatalf("%q should be recognized as Telegram command", text)
		}
	}
	for _, text := range []string{"", "开始使用", "hello /start"} {
		if isTelegramCommand(text) {
			t.Fatalf("%q should not be recognized as Telegram command", text)
		}
	}
}
