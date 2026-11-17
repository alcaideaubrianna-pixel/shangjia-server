package domain

import "testing"

func TestTelegramChatIDIsNotChannelRelationID(t *testing.T) {
	for _, id := range []TelegramChatID{-1003736052878, -1002258146662, -123456789, 123456789} {
		if !id.Valid() {
			t.Fatalf("valid Telegram chat rejected: %d", id)
		}
	}
	if TelegramChatID(0).Valid() {
		t.Fatal("zero is not a Telegram chat")
	}
	if ChannelID(-1003736052878).Valid() || ChannelID(0).Valid() || !ChannelID(17282290).Valid() {
		t.Fatal("internal channel relation ID validation changed")
	}
}
