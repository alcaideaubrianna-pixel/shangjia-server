package sys

import (
	"strings"

	"github.com/go-telegram/bot/models"
)

func telegramMessageMediaType(msg *models.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Video != nil {
		return "video"
	}
	if len(msg.Photo) > 0 {
		return "image"
	}
	return ""
}

func telegramFileIDMatchesMediaType(mediaType string, fileID string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return true
	}
	switch mediaType {
	case "video":
		return !strings.HasPrefix(fileID, "AgAC")
	case "image", "photo":
		return !strings.HasPrefix(fileID, "BAAC")
	default:
		return true
	}
}

func telegramMessageFileId(msg *models.Message) string {
	if msg == nil {
		return ""
	}
	if msg.Video != nil {
		return msg.Video.FileID
	}
	if len(msg.Photo) == 0 {
		return ""
	}
	best := msg.Photo[0]
	for _, item := range msg.Photo {
		if item.FileSize > best.FileSize {
			best = item
		}
	}
	return best.FileID
}
