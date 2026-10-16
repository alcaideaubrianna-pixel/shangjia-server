package runtime

import (
	"testing"

	"github.com/gotd/td/tg"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountMessagePhotoPreservesSelectedSize(t *testing.T) {
	for _, test := range []struct {
		name string
		size tg.PhotoSizeClass
		want int64
	}{
		{"regular", &tg.PhotoSize{Type: "y", W: 1000, H: 1000, Size: 12345}, 12345},
		{"progressive", &tg.PhotoSizeProgressive{Type: "y", W: 1000, H: 1000, Sizes: []int{100, 900, 500}}, 900},
		{"cached", &tg.PhotoCachedSize{Type: "y", W: 1000, H: 1000, Bytes: []byte{1, 2, 3}}, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := &tg.Message{ID: 10}
			message.SetMedia(&tg.MessageMediaPhoto{Photo: &tg.Photo{Sizes: []tg.PhotoSizeClass{
				&tg.PhotoSize{Type: "s", W: 10, H: 10, Size: 10}, test.size,
			}}})
			items := accountMessageMedia(message, "123")
			if len(items) != 1 || items[0].SourceThumbSize != "y" || items[0].SourceSize != test.want {
				t.Fatalf("unexpected photo metadata: %+v", items)
			}
		})
	}
}

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
