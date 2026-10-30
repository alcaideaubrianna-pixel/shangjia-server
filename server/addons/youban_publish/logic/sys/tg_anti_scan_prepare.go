package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/sync/errgroup"
)

// prepareTelegramAntiScanMediaSet is the job-level barrier for content
// protection. It is intentionally called only when the channel policy enables
// anti-scan. Every protected image (display and verify) is prepared before the
// first Telegram send is attempted.
func (s *sSysPublish) prepareTelegramAntiScanMediaSet(ctx context.Context, mediaSets ...[]*telegramMediaItem) error {
	items := make([]*telegramMediaItem, 0)
	for _, set := range mediaSets {
		for _, item := range set {
			if item != nil && item.AntiScanEnabled && isTelegramImageMedia(item.MediaType) {
				items = append(items, item)
			}
		}
	}
	if len(items) == 0 {
		return nil
	}
	g.Log().Infof(ctx, "防扫图全量准备开始 mediaCount:%d", len(items))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(telegramMediaPrepareConcurrency)
	for _, item := range items {
		item := item
		group.Go(func() error {
			path, cleanup, err := cachedTelegramMediaFile(groupCtx, item)
			if err != nil {
				return gerror.Wrapf(err, "防扫图源文件准备失败 mediaId:%d", item.Id)
			}
			path, cleanup, err = s.prepareTelegramMediaUploadFile(groupCtx, item, path, cleanup)
			if err != nil {
				if cleanup != nil {
					cleanup()
				}
				return gerror.Wrapf(err, "防扫图媒体处理失败 mediaId:%d", item.Id)
			}
			if path == "" {
				if cleanup != nil {
					cleanup()
				}
				return gerror.Newf("防扫图媒体处理结果为空 mediaId:%d", item.Id)
			}
			item.PreparedUploadPath = path
			item.PreparedUploadCleanup = cleanup
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		for _, item := range items {
			if item.PreparedUploadCleanup != nil {
				item.PreparedUploadCleanup()
				item.PreparedUploadCleanup = nil
			}
			item.PreparedUploadPath = ""
		}
		g.Log().Warningf(ctx, "防扫图全量准备失败 mediaCount:%d err:%+v", len(items), err)
		return err
	}
	g.Log().Infof(ctx, "防扫图全量准备完成 mediaCount:%d", len(items))
	return nil
}

func cleanupPreparedTelegramMedia(mediaSets ...[]*telegramMediaItem) {
	for _, set := range mediaSets {
		for _, item := range set {
			if item == nil {
				continue
			}
			if item.PreparedUploadCleanup != nil {
				item.PreparedUploadCleanup()
				item.PreparedUploadCleanup = nil
			}
			item.PreparedUploadPath = ""
		}
	}
}
