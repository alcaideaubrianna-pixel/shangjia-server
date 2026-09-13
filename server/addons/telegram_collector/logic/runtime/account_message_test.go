package runtime

import (
	"testing"

	"github.com/gotd/td/tg"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountMessageMediaNormalizesVideoDocuments(t *testing.T) {
	tests := []struct {
		name     string
		media    *tg.MessageMediaDocument
		wantType string
		wantMIME string
	}{
		{
			name:     "animation mp4",
			media:    &tg.MessageMediaDocument{Document: &tg.Document{ID: 1, MimeType: "video/mp4"}},
			wantType: sysin.MediaKindVideo, wantMIME: "video/mp4",
		},
		{
			name:     "ordinary document",
			media:    &tg.MessageMediaDocument{Document: &tg.Document{ID: 2, MimeType: "application/pdf"}},
			wantType: sysin.MediaKindFile, wantMIME: "application/pdf",
		},
		{
			name:     "telegram video flag",
			media:    &tg.MessageMediaDocument{Video: true, Document: &tg.Document{ID: 3, MimeType: "application/octet-stream"}},
			wantType: sysin.MediaKindVideo, wantMIME: "application/octet-stream",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.media.Video {
				test.media.SetVideo(true)
			}
			message := &tg.Message{ID: 10}
			message.SetMedia(test.media)
			items := accountMessageMedia(message, "123")
			if len(items) != 1 || items[0].Type != test.wantType || items[0].SourceMimeType != test.wantMIME {
				t.Fatalf("media=%+v", items)
			}
		})
	}
}
