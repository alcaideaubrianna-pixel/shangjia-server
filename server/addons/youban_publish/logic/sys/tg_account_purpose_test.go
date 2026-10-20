package sys

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestAccountMediaResponsePreservesPurpose(t *testing.T) {
	for _, purpose := range []string{"display", "verify"} {
		for _, short := range []bool{false, true} {
			media := []*telegramMediaItem{{Id: 4996314, Purpose: purpose}}
			var updates tg.UpdatesClass = &tg.Updates{Updates: []tg.UpdateClass{
				&tg.UpdateNewChannelMessage{Message: &tg.Message{ID: 45316, GroupedID: 123}},
			}}
			if short {
				updates = &tg.UpdateShortSentMessage{ID: 45316}
			}
			messages := gotdSentMessagesFromUpdates(updates, media)
			if len(messages) != 1 || messages[0].Purpose != purpose || messages[0].MediaId != media[0].Id {
				t.Fatalf("purpose=%s short=%t: incorrect response mapping: %+v", purpose, short, messages)
			}
			if err := validateTelegramMediaSendResult(purpose, media, messages); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestAccountVerifyRejectsDisplayResponse(t *testing.T) {
	media := []*telegramMediaItem{{Id: 4996314, Purpose: "verify"}}
	messages := []*telegramSentMessage{{MessageId: 45316, MediaId: 4996314, Purpose: "display"}}
	if err := validateTelegramMediaSendResult("verify", media, messages); err == nil {
		t.Fatal("verification response mislabeled as display was accepted")
	}
}

func TestAccountAlbumPreservesEachMediaPurpose(t *testing.T) {
	media := []*telegramMediaItem{{Id: 1, Purpose: "verify"}, {Id: 2, Purpose: "verify"}}
	first, second := &tg.Message{ID: 101}, &tg.Message{ID: 102}
	first.SetGroupedID(999)
	second.SetGroupedID(999)
	updates := &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateNewChannelMessage{Message: first},
		&tg.UpdateNewChannelMessage{Message: second},
	}}
	messages := gotdSentMessagesFromUpdates(updates, media)
	if err := validateTelegramMediaSendResult("verify", media, messages); err != nil {
		t.Fatal(err)
	}
	for i, message := range messages {
		if message.Purpose != "verify" || message.MediaId != media[i].Id || message.MediaGroupId != "999" {
			t.Fatalf("album identity lost: %+v", message)
		}
	}
}
