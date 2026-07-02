package sys

import "github.com/go-telegram/bot/models"

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
