package sys

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramSentMessagesFromGroupMatchesReturnedMediaType(t *testing.T) {
	media := []*telegramMediaItem{
		{Id: 101, MediaType: "video", AssetHash: "video-1"},
		{Id: 102, MediaType: "image", AssetHash: "image-1"},
		{Id: 103, MediaType: "video", AssetHash: "video-2"},
		{Id: 104, MediaType: "image", AssetHash: "image-2"},
	}
	messages := []*models.Message{
		{ID: 201, Photo: []models.PhotoSize{{FileID: "AgAC-photo-1"}}},
		{ID: 202, Video: &models.Video{FileID: "BAAC-video-1"}},
		{ID: 203, Photo: []models.PhotoSize{{FileID: "AgAC-photo-2"}}},
		{ID: 204, Video: &models.Video{FileID: "BAAC-video-2"}},
	}

	got := telegramSentMessagesFromGroup(messages, "display", media)
	if len(got) != 4 {
		t.Fatalf("unexpected result count: %d", len(got))
	}
	wantMediaIDs := []int64{102, 101, 104, 103}
	for index, item := range got {
		if item.MediaId != wantMediaIDs[index] {
			t.Fatalf("result %d media id: got %d want %d", index, item.MediaId, wantMediaIDs[index])
		}
		if !telegramFileIDMatchesMediaType(mediaTypeByID(media, item.MediaId), item.TgFileId) {
			t.Fatalf("result %d contains mismatched file id: mediaId=%d fileId=%s", index, item.MediaId, item.TgFileId)
		}
	}
}

func TestTelegramFileIDMatchesMediaTypeRejectsKnownCrossTypeIDs(t *testing.T) {
	if telegramFileIDMatchesMediaType("video", "AgAC-photo") {
		t.Fatal("photo file id must not be accepted as video")
	}
	if telegramFileIDMatchesMediaType("image", "BAAC-video") {
		t.Fatal("video file id must not be accepted as image")
	}
}

func mediaTypeByID(media []*telegramMediaItem, mediaID int64) string {
	for _, item := range media {
		if item != nil && item.Id == mediaID {
			return item.MediaType
		}
	}
	return ""
}
