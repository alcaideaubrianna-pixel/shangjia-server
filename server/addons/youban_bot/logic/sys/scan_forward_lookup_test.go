package sys

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"
	publishService "hotgo/addons/youban_publish/service"
)

type scanFingerprintCacheService struct {
	publishService.ISysPublish
}

func (*scanFingerprintCacheService) BotMediaFingerprintCached(context.Context, string) bool {
	return true
}

func TestCachedScanMediaSkipsTelegramResolution(t *testing.T) {
	var previous publishService.ISysPublish
	func() {
		defer func() { _ = recover() }()
		previous = publishService.SysPublish()
	}()
	publishService.RegisterSysPublish(&scanFingerprintCacheService{ISysPublish: previous})
	t.Cleanup(func() { publishService.RegisterSysPublish(previous) })
	for _, message := range []*models.Message{
		{Photo: []models.PhotoSize{{FileID: "photo", FileUniqueID: "stable-photo", FileSize: 100}}},
		{Video: &models.Video{FileUniqueID: "stable-video", Thumbnail: &models.PhotoSize{FileID: "thumbnail"}}},
	} {
		// A missing token would fail if the cached path called Telegram getFile.
		media, err := NewSysBot().resolveTelegramScanMedia(context.Background(), 0, "", message)
		if err != nil {
			t.Fatal(err)
		}
		items := scanSearchItems(media)
		if len(items) != 1 || items[0].FileUniqueId != telegramMessageFileUniqueId(message) || items[0].FileUrl != "" {
			t.Fatalf("cached scan media lost identity: %+v", items)
		}
	}
}

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

func TestTelegramMessageFileUniqueIdUsesLargestPhoto(t *testing.T) {
	msg := &models.Message{Photo: []models.PhotoSize{
		{FileUniqueID: "small", Width: 90, Height: 90, FileSize: 100},
		{FileUniqueID: "large", Width: 1280, Height: 720, FileSize: 1000},
	}}
	if got := telegramMessageFileUniqueId(msg); got != "large" {
		t.Fatalf("unexpected file_unique_id: %q", got)
	}
}

func TestScanPhotoSizeAvoidsLargestVariant(t *testing.T) {
	got := scanPhotoSize([]models.PhotoSize{
		{FileUniqueID: "thumb", Width: 90, Height: 90, FileSize: 100},
		{FileUniqueID: "medium", Width: 640, Height: 360, FileSize: 1000},
		{FileUniqueID: "large", Width: 1920, Height: 1080, FileSize: 8000},
	})
	if got.FileUniqueID != "medium" {
		t.Fatalf("unexpected scan photo size: %+v", got)
	}
}
