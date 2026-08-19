package sys

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramUpdateToWebhookInputPreservesChatMedia(t *testing.T) {
	update := &models.Update{
		ID: 91,
		Message: &models.Message{
			ID:              37,
			MessageThreadID: 12,
			Caption:         "media reply",
			Chat:            models.Chat{ID: -100123, Type: models.ChatTypeSupergroup},
			Photo:           []models.PhotoSize{{FileID: "photo-small"}, {FileID: "photo-large"}},
			Video:           &models.Video{FileID: "video-id", FileName: "clip.mp4", MimeType: "video/mp4"},
			Document:        &models.Document{FileID: "document-id", FileName: "notes.pdf", MimeType: "application/pdf"},
			Sticker:         &models.Sticker{FileID: "sticker-id", Emoji: "ok", IsVideo: true},
		},
	}

	in, err := telegramUpdateToWebhookInp(update)
	if err != nil {
		t.Fatalf("telegramUpdateToWebhookInp() error = %v", err)
	}
	if in == nil || in.UpdateId != 91 || in.Message == nil {
		t.Fatalf("unexpected input: %#v", in)
	}
	msg := in.Message
	if msg.MessageId != 37 || msg.MessageThreadId != 12 || msg.Chat == nil || msg.Chat.Id != -100123 {
		t.Fatalf("topic identity was not preserved: %#v", msg)
	}
	if len(msg.Photo) != 2 || msg.Photo[1].FileId != "photo-large" {
		t.Fatalf("photo data was not preserved: %#v", msg.Photo)
	}
	if msg.Video == nil || msg.Video.FileId != "video-id" || msg.Document == nil || msg.Document.FileId != "document-id" {
		t.Fatalf("file data was not preserved: video=%#v document=%#v", msg.Video, msg.Document)
	}
	if msg.Sticker == nil || msg.Sticker.FileId != "sticker-id" || !msg.Sticker.IsVideo {
		t.Fatalf("sticker data was not preserved: %#v", msg.Sticker)
	}
}

func TestTelegramUpdateToWebhookInputAcceptsChannelPost(t *testing.T) {
	update := &models.Update{
		ID: 92,
		ChannelPost: &models.Message{
			ID:              33,
			MessageThreadID: 4,
			Chat:            models.Chat{ID: -1004420178952, Type: models.ChatTypeSupergroup},
			Photo:           []models.PhotoSize{{FileID: "channel-photo"}},
		},
	}
	in, err := telegramUpdateToWebhookInp(update)
	if err != nil {
		t.Fatalf("telegramUpdateToWebhookInp() error = %v", err)
	}
	if in == nil || in.Message == nil || in.Message.MessageId != 33 {
		t.Fatalf("channel post was not mapped as a message: %#v", in)
	}
	if len(in.Message.Photo) != 1 || in.Message.Photo[0].FileId != "channel-photo" {
		t.Fatalf("channel post photo was not preserved: %#v", in.Message.Photo)
	}
}

func TestExternalOwnerIDUsesSeparateNegativeNamespace(t *testing.T) {
	if got := externalOwnerId(42); got != -42 {
		t.Fatalf("externalOwnerId() = %d, want -42", got)
	}
}
