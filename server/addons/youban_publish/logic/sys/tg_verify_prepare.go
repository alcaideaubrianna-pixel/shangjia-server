package sys

import (
	"context"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// Prepare upload bytes before creating a verify Attempt. Until that durable
// boundary the job remains display_confirmed and no verify request can exist.
func (s *sSysPublish) prepareTelegramVerifyUploads(ctx context.Context, media []*telegramMediaItem) (resultCleanup func(), resultErr error) {
	var cleanups []func()
	var once sync.Once
	cleanupAll := func() {
		once.Do(func() {
			for _, cleanup := range cleanups {
				cleanup()
			}
		})
	}
	defer func() {
		if resultErr != nil {
			cleanupAll()
		}
	}()
	for _, item := range media {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if item == nil || item.MediaType != "video" || item.PreparedUploadPath != "" {
			continue
		}
		s.prepareTelegramMediaItemForSend(ctx, item)
		if telegramVideoUsesReusableFileIdWithCover(item) ||
			(strings.TrimSpace(item.TgFileId) != "" && !telegramMediaRequiresSanitizedUpload(item)) ||
			telegramMediaRemoteURL(item) != "" {
			continue
		}
		stage := beginTelegramPublishStage(ctx, "verify.prepare_upload", telegramPublishMediaLogFields(item))
		path, cleanup, err := cachedTelegramMediaFile(ctx, item)
		if err == nil {
			path, cleanup, err = s.prepareTelegramMediaUploadFile(ctx, item, path, cleanup)
		}
		if err == nil && path == "" {
			err = gerror.New("验证视频文件准备结果为空")
		}
		if err == nil {
			err = validateTelegramMediaUploadSize(item, path)
		}
		stage.End(err)
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return nil, gerror.Wrap(err, "准备验证视频失败")
		}
		item.PreparedUploadPath = path
		// Upload attempts and Account fallbacks may shallow-copy the item.
		// Only the outer verify operation owns deletion; each upload closes
		// its own file descriptor without removing these shared bytes.
		item.PreparedUploadCleanup = nil
		if cleanup != nil {
			cleanups = append(cleanups, cleanup)
		}
		g.Log().Info(ctx, "验证视频文件准备完成，尚未创建发送Attempt", g.Map{
			"event": "telegram_verify_prepared", "jobId": item.JobId,
			"mediaId": item.Id, "sizeBytes": fileSizeValue(path),
		})
	}
	return cleanupAll, nil
}
