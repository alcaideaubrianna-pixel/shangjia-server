package sys

import (
	"os"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

const telegramMediaUploadSizeError = "媒体超过官方Bot API上传限制"

// Conservative decimal limits leave margin below Telegram's documented MB
// limits. Unknown legacy sizes retain URL-first delivery with deterministic
// rejection fallback; never issue HEAD requests on the publishing hot path.
func telegramMediaWithinURLSizeLimit(media *telegramMediaItem) bool {
	if media == nil {
		return false
	}
	limit := int64(20_000_000)
	if telegramMediaIsPhoto(media) {
		limit = 5_000_000
	}
	return media.Size <= 0 || media.Size <= limit
}

func telegramMediaIsPhoto(media *telegramMediaItem) bool {
	return strings.EqualFold(media.MediaType, "image") || strings.EqualFold(media.MediaType, "photo")
}

// Check the actual processed bytes, not stale DB metadata. This path stays
// compatible with official sendPhoto/sendVideo; it never sends a Document.
func validateTelegramMediaUploadSize(media *telegramMediaItem, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return gerror.Wrap(err, "读取待上传媒体大小失败")
	}
	limit := int64(50_000_000)
	if telegramMediaIsPhoto(media) {
		limit = 10_000_000
	}
	if info.Size() > limit {
		return gerror.Newf("%s，请压缩后重试 sizeBytes:%d limitBytes:%d", telegramMediaUploadSizeError, info.Size(), limit)
	}
	return nil
}
