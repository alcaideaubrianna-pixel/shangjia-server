package sysin

import (
	"testing"

	"hotgo/addons/telegram_collector/domain"
)

func TestRawUpdateBoundaryTelegramChats(t *testing.T) {
	for _, chatID := range []int64{-1003736052878, -1002258146662, -123456789, 123456789} {
		event := RawUpdateEvent{TenantID: 78, SourceID: 351, SourceType: SourceTypeBot, ChatID: chatID, MessageID: 8928}
		_, _, _, typedChatID, _ := event.RawUpdateIDs()
		var remoteID domain.TelegramChatID = typedChatID
		if !remoteID.Valid() || !event.ValidateBoundary() {
			t.Fatalf("Bot update rejected for chat %d", chatID)
		}
		event.SourceType, event.AccountID = SourceTypeAccount, 104
		if !event.ValidateBoundary() {
			t.Fatalf("account update rejected for chat %d", chatID)
		}
	}
	// A non-message update can legitimately omit chat/message identity.
	event := RawUpdateEvent{TenantID: 78, SourceID: 351, SourceType: SourceTypeBot}
	if !event.ValidateBoundary() {
		t.Fatal("non-message Bot update rejected")
	}
	event.ChatID, event.MessageID = -1003736052878, -1
	if event.ValidateBoundary() {
		t.Fatal("negative message ID accepted")
	}
	event.MessageID, event.SourceType = 8928, SourceTypeAccount
	if event.ValidateBoundary() {
		t.Fatal("account update without protocol account accepted")
	}
}
