package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/sync/errgroup"
)

const (
	telegramAntiScanPrepareTimeout     = 90 * time.Second
	telegramAntiScanBackgroundTimeout  = 2 * time.Minute
	telegramAntiScanLightweightTimeout = 2 * time.Minute
)

// prepareTelegramAntiScanMediaSet is the job-level barrier for content
// protection. It is intentionally called only when the channel policy enables
// anti-scan. Every protected image (display and verify) is prepared before the
// first Telegram send is attempted.
func (s *sSysPublish) prepareTelegramAntiScanMediaSet(ctx context.Context, mediaSets ...[]*telegramMediaItem) error {
	return prepareTelegramAntiScanMediaSetWithTimeout(ctx, telegramAntiScanPrepareTimeout, s.prepareTelegramAntiScanMediaItem, mediaSets...)
}

func (s *sSysPublish) prepareTelegramAntiScanMediaItem(ctx context.Context, item *telegramMediaItem) error {
	path, cleanup, err := cachedTelegramMediaFile(ctx, item)
	if err != nil {
		return gerror.Wrapf(err, "防扫图源文件准备失败 mediaId:%d", item.Id)
	}
	path, cleanup, err = s.prepareTelegramMediaUploadFile(ctx, item, path, cleanup)
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
}

func prepareTelegramAntiScanMediaSetWithTimeout(
	ctx context.Context,
	timeout time.Duration,
	prepare func(context.Context, *telegramMediaItem) error,
	mediaSets ...[]*telegramMediaItem,
) error {
	prepareCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
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
	// One damaged image must not cancel sibling preparation. The caller falls
	// back the whole album after Wait, while the shared parent deadline still
	// bounds every worker. A fail-fast group caused cancelled siblings to run
	// their candidate retry loops repeatedly before the fallback could proceed.
	group := new(errgroup.Group)
	group.SetLimit(telegramMediaPrepareConcurrency)
	for _, item := range items {
		item := item
		group.Go(func() error {
			return prepare(prepareCtx, item)
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

func fallbackTelegramAntiScanToOriginal(mediaSets ...[]*telegramMediaItem) {
	cleanupPreparedTelegramMedia(mediaSets...)
	for _, set := range mediaSets {
		for _, item := range set {
			if item == nil {
				continue
			}
			item.AntiScanEnabled = false
			item.ForceUpload = false
		}
	}
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
