package sys

import (
	"encoding/json"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestInlineQueryReplyMarkupMarshalsAsObject(t *testing.T) {
	markup := inlineQueryReplyMarkup(&models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "测试", URL: "https://t.me/test"}}}})
	data, err := json.Marshal(markup)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[0] != '{' {
		t.Fatalf("inline reply markup must be a JSON object: %s", data)
	}
}

func TestInlineQueryResultOmitsEmptyReplyMarkup(t *testing.T) {
	result := models.InlineQueryResultArticle{
		ID:          "XX1V01S0",
		Title:       "频道推广",
		ReplyMarkup: inlineQueryReplyMarkup(nil),
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || json.Valid(data) == false {
		t.Fatalf("invalid inline result JSON: %s", data)
	}
	var decoded map[string]interface{}
	if err = json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, exists := decoded["reply_markup"]; exists {
		t.Fatalf("empty reply markup must be omitted: %s", data)
	}
}

func TestTemplateInlineButtonURLNormalizesUsername(t *testing.T) {
	if got := templateInlineButtonURL("@xhjsjbot"); got != "https://t.me/xhjsjbot" {
		t.Fatalf("unexpected normalized URL: %s", got)
	}
}

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
