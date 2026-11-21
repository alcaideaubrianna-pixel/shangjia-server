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

var telegramAntiScanPrepareSlots = make(chan struct{}, channelQueueWorkerConcurrency())

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
	var originals, items []*telegramMediaItem
	for _, set := range mediaSets {
		for _, item := range set {
			if item != nil && item.AntiScanEnabled && isTelegramImageMedia(item.MediaType) {
				copy := *item
				copy.PreparedUploadPath = ""
				copy.PreparedUploadCleanup = nil
				originals = append(originals, item)
				items = append(items, &copy)
			}
		}
	}
	if len(items) == 0 {
		return nil
	}
	g.Log().Infof(ctx, "防扫图全量准备开始 mediaCount:%d", len(items))
	// A timed-out worker owns its copies and temporary files until it exits.
	// Only a complete result can transfer ownership to the sender.
	// Keep the same process-wide ceiling even when a timed-out job leaves
	// image work running while its channel moves on.
	select {
	case telegramAntiScanPrepareSlots <- struct{}{}:
	case <-prepareCtx.Done():
		return prepareCtx.Err()
	}
	done := make(chan error)
	go func() {
		defer func() { <-telegramAntiScanPrepareSlots }()
		group := new(errgroup.Group)
		group.SetLimit(telegramMediaPrepareConcurrency)
		for _, item := range items {
			if prepareCtx.Err() != nil {
				break
			}
			item := item
			group.Go(func() error {
				if err := prepareCtx.Err(); err != nil {
					return err
				}
				return prepare(prepareCtx, item)
			})
		}
		err := group.Wait()
		select {
		case done <- err:
		case <-prepareCtx.Done():
			cleanupPreparedTelegramMedia(items)
		}
	}()
	select {
	case err := <-done:
		if err == nil {
			err = prepareCtx.Err()
		}
		if err != nil {
			cleanupPreparedTelegramMedia(items)
			g.Log().Warningf(ctx, "防扫图全量准备失败 mediaCount:%d err:%+v", len(items), err)
			return err
		}
	case <-prepareCtx.Done():
		return prepareCtx.Err()
	}
	cleanupPreparedTelegramMedia(originals)
	for i, item := range items {
		*originals[i] = *item
	}
	g.Log().Infof(ctx, "防扫图全量准备完成 mediaCount:%d", len(items))
	return nil
}

func fallbackTelegramAntiScanToOriginal(mediaSets ...[]*telegramMediaItem) {
	cleanupPreparedTelegramMedia(mediaSets...)
	for _, set := range mediaSets {
		for _, item := range set {
			if item == nil || !item.AntiScanEnabled {
				continue
			}
			item.AntiScanEnabled = false
			item.ForceUpload = false
			item.ProcessedAssetHash = ""
			item.ProtectedHashKey = ""
			item.ProtectedPHash = 0
			item.ProtectedDHash = 0
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
