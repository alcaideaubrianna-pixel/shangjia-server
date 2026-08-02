package sys

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestDecodeTemplateInlineCachedPhoto(t *testing.T) {
	raw := `{"photo":[{"file_id":"small","width":90,"height":90},{"file_id":"largest","width":1280,"height":720}],"caption":"测试文案","caption_entities":[{"type":"custom_emoji","offset":0,"length":2,"custom_emoji_id":"5976568857786062743"}]}`
	photo := decodeTemplateInlineCachedPhoto(raw)
	if photo == nil {
		t.Fatal("expected cached photo")
	}
	if photo.FileID != "largest" {
		t.Fatalf("unexpected file id: %s", photo.FileID)
	}
	if photo.Caption != "测试文案" {
		t.Fatalf("unexpected caption: %s", photo.Caption)
	}
	if len(photo.CaptionEntities) != 1 || photo.CaptionEntities[0].Type != models.MessageEntityTypeCustomEmoji || photo.CaptionEntities[0].CustomEmojiID != "5976568857786062743" {
		t.Fatalf("custom emoji entity was not preserved: %+v", photo.CaptionEntities)
	}
}
