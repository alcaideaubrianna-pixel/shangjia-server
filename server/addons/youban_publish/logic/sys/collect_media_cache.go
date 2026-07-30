package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/entity"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/hgrds/lock"
	"hotgo/internal/model"
)

type collectMediaRetryError struct {
	message     string
	delay       time.Duration
	rateLimited bool
}

type collectMediaDiscardedError struct {
	message string
}

func (e *collectMediaDiscardedError) Error() string {
	return e.message
}

const defaultCollectMediaDownloadThreads = 4

func (e *collectMediaRetryError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func newCollectMediaRetryError(message string, delay time.Duration) *collectMediaRetryError {
	if delay <= 0 {
		delay = 15 * time.Second
	}
	return &collectMediaRetryError{message: message, delay: delay}
}

func newCollectMediaRateLimitError(err error) *collectMediaRetryError {
	delay := collectMediaRateLimitDelay(err)
	return &collectMediaRetryError{
		message:     fmt.Sprintf("TG媒体下载触发限流，等待%s后自动重试：%v", delay.Round(time.Second), err),
		delay:       delay,
		rateLimited: true,
	}
}

func collectMediaRateLimitDelay(err error) time.Duration {
	delay := time.Minute
	if floodWait, ok := tgerr.AsFloodWait(err); ok && floodWait > 0 {
		delay = floodWait
	}
	if delay < 30*time.Second {
		delay = 30 * time.Second
	}
	if delay > 2*time.Hour {
		delay = 2 * time.Hour
	}
	return delay
}

func collectMediaRetryErrorFrom(err error) *collectMediaRetryError {
	if err == nil {
		return nil
	}
	var retryErr *collectMediaRetryError
	if errors.As(err, &retryErr) {
		return retryErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return newCollectMediaRetryError("账号采集媒体下载临时中断，等待重试: "+err.Error(), 30*time.Second)
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "file_reference_expired") {
		return nil
	}
	if _, ok := tgerr.AsFloodWait(err); ok || strings.Contains(message, "too many requests") || strings.Contains(message, "flood_wait") {
		return newCollectMediaRateLimitError(err)
	}
	retryablePatterns := []string{
		"auth_bytes_invalid",
		"file_migrate",
		"context canceled",
		"context deadline exceeded",
		"deadline exceeded",
		"timeout",
		"connection reset",
		"connection refused",
		"connection closed",
		"dc is closed",
		"broken pipe",
		"temporary",
		"eof",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(message, pattern) {
			return newCollectMediaRetryError("账号采集媒体下载临时失败，等待重试: "+err.Error(), 30*time.Second)
		}
	}
	return nil
}

func (s *sSysPublish) prepareCollectMediaAsset(ctx context.Context, event gdb.Record, item collectMediaItem) (collectMediaItem, error) {
	if !strings.HasPrefix(strings.TrimSpace(item.FileId), "gotd:") {
		return item, nil
	}
	if strings.TrimSpace(item.StoragePath) != "" || strings.TrimSpace(item.FileUrl) != "" {
		item.FileId = ""
		return item, nil
	}
	return item, gerror.New("账号采集媒体尚未缓存")
}

func (s *sSysPublish) ExecuteCollectMediaCache(ctx context.Context, payload collectMediaQueuePayload) (retErr error) {
	taskStartedAt := time.Now()
	var statEvent gdb.Record
	defer func() {
		if !statEvent.IsEmpty() {
			s.recordCollectMediaPerformance(context.Background(), statEvent, taskStartedAt, retErr)
		}
	}()
	ctx = collectMediaRuntimeContext(ctx, payload.AccountId)
	g.Log().Infof(ctx, "采集媒体缓存任务开始 eventId:%d tenantId:%d accountId:%d sourceId:%d tgAccountId:%d", payload.EventId, payload.TenantId, payload.AccountId, payload.SourceId, payload.TgAccountId)
	lockStartedAt := time.Now()
	distributedLock := lock.NewConfig(35*time.Minute, time.Second).Mutex(fmt.Sprintf("youban_publish:collect:media:event:%d", payload.EventId))
	if err := distributedLock.TryLock(ctx); err != nil {
		if errors.Is(err, lock.ErrLockFailed) {
			g.Log().Debugf(ctx, "采集媒体任务检测到重复执行，跳过当前任务 eventId:%d wait:%s", payload.EventId, time.Since(lockStartedAt).Round(time.Millisecond))
			return nil
		}
		g.Log().Errorf(ctx, "采集媒体任务获取事件锁异常 eventId:%d wait:%s err:%+v", payload.EventId, time.Since(lockStartedAt).Round(time.Millisecond), err)
		return newCollectMediaRetryError("获取采集事件媒体锁失败: "+err.Error(), 15*time.Second)
	}
	g.Log().Infof(ctx, "采集媒体任务获取事件锁完成 eventId:%d wait:%s", payload.EventId, time.Since(lockStartedAt).Round(time.Millisecond))
	defer func() { _ = distributedLock.Unlock(context.Background()) }()
	accountIntervalStartedAt := time.Now()
	s.waitCollectMediaAccountInterval(ctx, payload.TenantId, payload.TgAccountId)
	g.Log().Infof(ctx, "采集媒体任务账号间隔等待完成 eventId:%d wait:%s", payload.EventId, time.Since(accountIntervalStartedAt).Round(time.Millisecond))
	readEventStartedAt := time.Now()
	event, err := s.collectMediaCacheEvent(ctx, payload)
	if err != nil {
		return err
	}
	g.Log().Infof(ctx, "采集媒体任务读取事件完成 eventId:%d duration:%s status:%s", payload.EventId, time.Since(readEventStartedAt).Round(time.Millisecond), event["status"].String())
	if event.IsEmpty() {
		g.Log().Warningf(ctx, "采集媒体任务对应事件不存在，跳过历史任务 eventId:%d tenantId:%d accountId:%d sourceId:%d", payload.EventId, payload.TenantId, payload.AccountId, payload.SourceId)
		return nil
	}
	statEvent = event
	if collectEventAlreadyMatched(event["status"].String()) {
		g.Log().Infof(ctx, "采集媒体任务对应事件已完成，跳过重复任务 eventId:%d status:%s", payload.EventId, event["status"].String())
		return nil
	}
	s.appendCollectEventLogForRecord(ctx, event, "media", "running", "媒体缓存任务开始执行", "")
	if delay := s.collectGroupedMediaCacheDelay(ctx, event); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	cacheStartedAt := time.Now()
	changed, err := s.cacheCollectEventStructuredMedia(ctx, event)
	g.Log().Infof(ctx, "采集媒体任务媒体阶段完成 eventId:%d duration:%s changed:%t err:%v", payload.EventId, time.Since(cacheStartedAt).Round(time.Millisecond), changed, err)
	if err != nil {
		var discardedErr *collectMediaDiscardedError
		if errors.As(err, &discardedErr) {
			g.Log().Infof(ctx, "采集媒体引用已过期，已丢弃整组资料 eventId:%d reason:%s", payload.EventId, discardedErr.message)
			return nil
		}
		if retryErr, ok := err.(*collectMediaRetryError); ok {
			_ = s.markCollectEvent(ctx, payload.EventId, sysin.CollectEventStatusMediaPending, retryErr.message)
			status := "pending"
			stage := "media"
			if retryErr.rateLimited {
				status = "rate_limited"
				stage = "rate_limit"
			}
			s.appendCollectEventLogForRecord(ctx, event, stage, status, retryErr.message, fmt.Sprintf("retryAfter=%s", retryErr.delay))
			return retryErr
		}
		g.Log().Errorf(ctx, "账号采集媒体缓存失败 eventId:%d tenantId:%d accountId:%d sourceId:%d err:%+v", payload.EventId, payload.TenantId, payload.AccountId, payload.SourceId, err)
		_ = s.markCollectEvent(ctx, payload.EventId, sysin.CollectEventStatusFailed, err.Error())
		s.appendCollectEventLogForRecord(ctx, event, "media", "failed", err.Error(), "")
		return err
	}
	if changed {
		if err = s.syncCollectEventMediaSnapshot(ctx, payload.EventId); err != nil {
			g.Log().Errorf(ctx, "同步采集媒体快照失败 eventId:%d err:%+v", payload.EventId, err)
			return err
		}
	}
	g.Log().Infof(ctx, "采集媒体缓存任务完成 eventId:%d changed:%t total:%s", payload.EventId, changed, time.Since(taskStartedAt).Round(time.Millisecond))
	s.appendCollectEventLogForRecord(ctx, event, "media", "ready", "媒体缓存任务处理完成", "")
	if err := s.processCollectEvent(ctx, payload.EventId, payload.TenantId, payload.AccountId); err != nil {
		g.Log().Errorf(ctx, "媒体缓存完成后继续处理采集事件失败 eventId:%d err:%+v", payload.EventId, err)
		return err
	}
	return nil
}

func collectMediaRuntimeContext(ctx context.Context, accountId int64) context.Context {
	current := contexts.Get(ctx)
	if current == nil {
		return context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{
			Module:    consts.AppApi,
			AddonName: "youban_publish",
			User: &model.Identity{
				Id:  accountId,
				App: consts.AppApi,
			},
			Data: g.Map{},
		})
	}
	if current.Module == "" {
		current.Module = consts.AppApi
	}
	if current.AddonName == "" {
		current.AddonName = "youban_publish"
	}
	if current.User == nil || current.User.Id <= 0 {
		current.User = &model.Identity{Id: accountId, App: consts.AppApi}
	}
	if current.User.App == "" {
		current.User.App = consts.AppApi
	}
	return ctx
}

func (s *sSysPublish) collectMediaCacheEvent(ctx context.Context, payload collectMediaQueuePayload) (gdb.Record, error) {
	row, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", payload.EventId).
		Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集媒体缓存事件失败")
	}
	if row.IsEmpty() {
		return gdb.Record{}, nil
	}
	return row, nil
}

func (s *sSysPublish) collectGroupedMediaCacheDelay(ctx context.Context, event gdb.Record) time.Duration {
	if event.IsEmpty() || strings.TrimSpace(event["source_grouped_id"].String()) == "" {
		return 0
	}
	ingestedAt := event["received_at"].GTime()
	if ingestedAt == nil {
		ingestedAt = event["created_at"].GTime()
	}
	if ingestedAt == nil {
		return 0
	}
	elapsed := collectLocalElapsedSince(ingestedAt)
	if elapsed < 0 {
		return 0
	}
	if elapsed >= collectGroupedEventDelay {
		return 0
	}
	return collectGroupedEventDelay - elapsed + 500*time.Millisecond
}

func collectLocalElapsedSince(value *gtime.Time) time.Duration {
	if value == nil {
		return 0
	}
	wallClock := value.Time
	localTime := time.Date(
		wallClock.Year(),
		wallClock.Month(),
		wallClock.Day(),
		wallClock.Hour(),
		wallClock.Minute(),
		wallClock.Second(),
		wallClock.Nanosecond(),
		time.Local,
	)
	return time.Since(localTime)
}

func (s *sSysPublish) cacheCollectEventStructuredMedia(ctx context.Context, event gdb.Record) (bool, error) {
	stageStartedAt := time.Now()
	rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
	if err != nil {
		return false, err
	}
	items := collectMediaRowsToItems(rows, event["material_role"].String())
	g.Log().Infof(ctx, "采集媒体阶段读取媒体完成 eventId:%d sourceType:%s mediaCount:%d duration:%s", event["id"].Int64(), event["source_type"].String(), len(rows), time.Since(stageStartedAt).Round(time.Millisecond))
	s.appendCollectEventLogForRecord(ctx, event, "media", "checking", "开始检查媒体缓存方式", fmt.Sprintf("media=%d", len(items)))
	changed := false
	if event["source_type"].String() == sysin.CollectSourceTypeAccount {
		s.appendCollectEventLogForRecord(ctx, event, "media", "downloading", "账号采集媒体使用下载缓存，保证带文案媒体组可原格式发送", "")
	} else {
		forwarded, forwardChanged, err := s.forwardCollectMediaToBackup(ctx, event, items)
		if err != nil {
			if retryErr, ok := err.(*collectMediaRetryError); ok {
				s.appendCollectEventLogForRecord(ctx, event, "media", "downloading", "备份频道转存暂不可用，改用下载兜底", retryErr.Error())
			} else {
				return false, err
			}
		}
		if forwardChanged {
			s.appendCollectEventLogForRecord(ctx, event, "media", "forwarded", "媒体已转存到备份频道", "")
			return s.updateCollectEventMediaItems(ctx, rows, forwarded)
		}
	}
	downloadRows := make([]*entity.YoubanPublishCollectEventMedia, 0, len(rows))
	downloadedItems := make([]collectMediaItem, 0, len(rows))
	type downloadResult struct {
		index int
		row   *entity.YoubanPublishCollectEventMedia
		item  collectMediaItem
		err   error
	}
	results := make([]downloadResult, len(rows))
	fileSlots := make(chan struct{}, accountCollectMediaConcurrency(ctx))
	var downloadWait sync.WaitGroup
	for index, row := range rows {
		if row == nil || !collectMediaRowNeedsCache(row.SourceFileId, row.SourceMessageRef, row.StoragePath, row.FileUrl, row.BackupChatId, row.BackupMessageId) {
			continue
		}
		reuseStartedAt := time.Now()
		reused, reuseErr := s.reuseCollectMediaCache(ctx, row)
		if reuseErr != nil {
			return changed, reuseErr
		}
		g.Log().Debugf(ctx, "采集媒体缓存复用检查 eventId:%d mediaId:%d sourceMessageId:%d reused:%t duration:%s", event["id"].Int64(), row.Id, row.SourceMessageId, reused, time.Since(reuseStartedAt).Round(time.Millisecond))
		if reused {
			cachedBytes, _ := fileSize(row.StoragePath)
			_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
				"cache_hit":            1,
				"download_duration_ms": 0,
				"download_bytes":       cachedBytes,
				"download_error_type":  "",
				"updated_at":           gtime.Now(),
			}).Update()
			items[index].FileUrl = row.FileUrl
			items[index].StoragePath = row.StoragePath
			items[index].PosterUrl = row.PosterUrl
			changed = true
			continue
		}
		downloadWait.Add(1)
		go func(index int, row *entity.YoubanPublishCollectEventMedia) {
			defer downloadWait.Done()
			startedAt := time.Now()
			result := downloadResult{index: index, row: row}
			slotStartedAt := time.Now()
			select {
			case fileSlots <- struct{}{}:
			case <-ctx.Done():
				result.err = ctx.Err()
				results[index] = result
				return
			}
			g.Log().Debugf(ctx, "采集媒体下载获取并发槽完成 eventId:%d mediaId:%d wait:%s", event["id"].Int64(), row.Id, time.Since(slotStartedAt).Round(time.Millisecond))
			defer func() { <-fileSlots }()
			accountSlotStartedAt := time.Now()
			releaseAccountSlot, slotErr := s.acquireCollectMediaDownloadSlots(ctx, event["tenant_id"].Int64(), event["tg_account_id"].Int64())
			if slotErr != nil {
				result.err = slotErr
				results[index] = result
				return
			}
			defer releaseAccountSlot()
			g.Log().Debugf(ctx, "采集媒体下载获取全局/账号并发槽完成 eventId:%d mediaId:%d wait:%s", event["id"].Int64(), row.Id, time.Since(accountSlotStartedAt).Round(time.Millisecond))
			statusStartedAt := time.Now()
			_, statusErr := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
				"cache_status":          collectMediaCacheDownloading,
				"cache_hit":             0,
				"download_attempts":     gdb.Raw("download_attempts+1"),
				"download_error_type":   "",
				collectMediaNextRetryAt: nil,
				"error_message":         "",
				"updated_at":            gtime.Now(),
			}).Update()
			if statusErr != nil {
				g.Log().Warningf(ctx, "采集媒体更新下载中状态失败 eventId:%d mediaId:%d duration:%s err:%+v", event["id"].Int64(), row.Id, time.Since(statusStartedAt).Round(time.Millisecond), statusErr)
			} else {
				g.Log().Debugf(ctx, "采集媒体更新下载中状态完成 eventId:%d mediaId:%d duration:%s", event["id"].Int64(), row.Id, time.Since(statusStartedAt).Round(time.Millisecond))
			}
			s.appendCollectEventLogForRecord(ctx, event, "media", "downloading", "开始下载账号采集媒体", fmt.Sprintf("mediaId=%d sourceMessageId=%d sourceFileId=%s", row.Id, row.SourceMessageId, row.SourceFileId))
			cached, err := s.downloadTelegramMedia(ctx, event["tenant_id"].Int64(), event["tg_account_id"].Int64(), items[index])
			if err != nil {
				downloadDuration := time.Since(startedAt).Milliseconds()
				errorType := collectMediaErrorType(err.Error())
				retryErr := collectMediaRetryErrorFrom(err)
				if retryErr != nil {
					g.Log().Warningf(ctx, "账号采集媒体下载暂不可用，等待自动重试 eventId:%d mediaId:%d sourceMessageId:%d duration:%s err:%+v", event["id"].Int64(), row.Id, row.SourceMessageId, time.Since(startedAt).Round(time.Millisecond), err)
				} else {
					g.Log().Errorf(ctx, "账号采集媒体下载失败 eventId:%d mediaId:%d sourceMessageId:%d duration:%s err:%+v", event["id"].Int64(), row.Id, row.SourceMessageId, time.Since(startedAt).Round(time.Millisecond), err)
				}
				if collectMediaAuthBytesInvalid(err) {
					s.restartAccountCollectWorker(ctx, event["tg_account_id"].Int64(), err)
				}
				if retryErr != nil {
					_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
						"cache_status":          collectMediaCachePending,
						"download_duration_ms":  downloadDuration,
						"download_error_type":   errorType,
						"error_message":         retryErr.message,
						collectMediaNextRetryAt: gtime.Now().Add(retryErr.delay),
						"updated_at":            gtime.Now(),
					}).Update()
					result.err = retryErr
				} else {
					_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
						"cache_status":          collectMediaCacheFailed,
						"download_duration_ms":  downloadDuration,
						"download_error_type":   errorType,
						"error_message":         err.Error(),
						collectMediaNextRetryAt: nil,
						"updated_at":            gtime.Now(),
					}).Update()
					result.err = err
				}
				results[index] = result
				return
			}
			if cached == nil || strings.TrimSpace(cached.Path) == "" {
				err = gerror.New("账号采集媒体下载未返回有效缓存文件")
				g.Log().Errorf(ctx, "账号采集媒体下载返回空结果 eventId:%d mediaId:%d sourceMessageId:%d duration:%s", event["id"].Int64(), row.Id, row.SourceMessageId, time.Since(startedAt).Round(time.Millisecond))
				_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
					"cache_status":          collectMediaCacheFailed,
					"error_message":         err.Error(),
					collectMediaNextRetryAt: nil,
					"updated_at":            gtime.Now(),
				}).Update()
				result.err = err
				results[index] = result
				return
			}
			cachedSize, _ := fileSize(cached.Path)
			downloadDuration := time.Since(startedAt).Milliseconds()
			_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
				"download_duration_ms": downloadDuration,
				"download_bytes":       cachedSize,
				"download_error_type":  "",
				"updated_at":           gtime.Now(),
			}).Update()
			g.Log().Infof(ctx, "账号采集媒体下载完成 eventId:%d mediaId:%d sourceMessageId:%d duration:%s size:%d", event["id"].Int64(), row.Id, row.SourceMessageId, time.Since(startedAt).Round(time.Millisecond), cachedSize)
			result.item = collectMediaItem{
				Type:        items[index].Type,
				FileUrl:     cached.FileUrl,
				StoragePath: cached.Path,
				PosterUrl:   items[index].PosterUrl,
				MetaJson:    strings.TrimSpace(cached.MetaJson),
			}
			if result.item.MetaJson == "" {
				result.item.MetaJson = items[index].MetaJson
			}
			g.Log().Debugf(ctx, "采集媒体下载结果准备写回 eventId:%d mediaId:%d duration:%s size:%d", event["id"].Int64(), row.Id, time.Since(startedAt).Round(time.Millisecond), cachedSize)
			results[index] = result
		}(index, row)
	}
	downloadWait.Wait()
	for _, result := range results {
		if result.row != nil && result.err != nil && collectMediaFileReferenceExpired(result.err) {
			reason := "TG媒体文件引用已过期，整组资料已丢弃"
			if discardErr := s.discardCollectEventGroup(ctx, event["id"].Int64(), reason); discardErr != nil {
				return changed, discardErr
			}
			return changed, &collectMediaDiscardedError{message: reason}
		}
	}
	for _, result := range results {
		if result.row == nil {
			continue
		}
		if result.err != nil {
			return changed, result.err
		}
		downloadRows = append(downloadRows, result.row)
		downloadedItems = append(downloadedItems, result.item)
	}
	if len(downloadRows) == 0 {
		return changed, nil
	}
	backupItems, uploadedToBackup, backupErr := s.cacheDownloadedCollectMediaGroupToBackup(ctx, event, downloadedItems)
	if backupErr != nil {
		s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "下载媒体组推送备份频道失败，改用目标频道直接上传", backupErr.Error())
	}
	if uploadedToBackup {
		downloadedItems = backupItems
	}
	for index, row := range downloadRows {
		cachedItem := downloadedItems[index]
		_, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
			"source_file_id":    cachedItem.FileId,
			"source_ref_type":   collectMediaRefType(cachedItem),
			"file_url":          cachedItem.FileUrl,
			"storage_path":      cachedItem.StoragePath,
			"backup_chat_id":    collectMediaBackupChatId(cachedItem),
			"backup_message_id": collectMediaBackupMessageId(cachedItem),
			"cache_status":      collectMediaCacheReady,
			"error_message":     "",
			"updated_at":        gtime.Now(),
		}).Update()
		if err != nil {
			return changed, gerror.Wrap(err, "更新采集媒体下载结果失败")
		}
		changed = true
	}
	return changed, nil
}

func (s *sSysPublish) reuseCollectMediaCache(ctx context.Context, row *entity.YoubanPublishCollectEventMedia) (bool, error) {
	if row == nil || strings.TrimSpace(row.SourceChatId) == "" || row.SourceMessageId <= 0 || strings.TrimSpace(row.SourceMediaKey) == "" {
		return false, nil
	}
	cols := pdao.YoubanPublishCollectEventMedia.Columns()
	candidates, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields(cols.FileUrl, cols.StoragePath, cols.PosterUrl).
		Where(cols.SourceChatId, row.SourceChatId).
		Where(cols.SourceMessageId, row.SourceMessageId).
		Where(cols.SourceMediaKey, row.SourceMediaKey).
		Where(cols.CacheStatus, collectMediaCacheReady).
		WhereNot(cols.Id, row.Id).
		OrderDesc(cols.UpdatedAt).
		Limit(10).
		All()
	if err != nil {
		return false, gerror.Wrap(err, "读取可复用采集媒体缓存失败")
	}
	for _, candidate := range candidates {
		fileURL := strings.TrimSpace(candidate[cols.FileUrl].String())
		storagePath := strings.TrimSpace(candidate[cols.StoragePath].String())
		if fileURL == "" && storagePath == "" {
			continue
		}
		if fileURL == "" && storagePath != "" && !fileExists(resolveTelegramLocalPath(storagePath)) {
			continue
		}
		data := g.Map{
			cols.FileUrl:      fileURL,
			cols.StoragePath:  storagePath,
			cols.PosterUrl:    strings.TrimSpace(candidate[cols.PosterUrl].String()),
			cols.CacheStatus:  collectMediaCacheReady,
			cols.ErrorMessage: "",
			cols.UpdatedAt:    gtime.Now(),
		}
		if _, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where(cols.Id, row.Id).Data(data).Update(); err != nil {
			return false, gerror.Wrap(err, "写入复用采集媒体缓存失败")
		}
		row.FileUrl = fileURL
		row.StoragePath = storagePath
		row.PosterUrl = strings.TrimSpace(candidate[cols.PosterUrl].String())
		row.CacheStatus = collectMediaCacheReady
		row.ErrorMessage = ""
		g.Log().Infof(ctx, "复用采集媒体缓存 mediaId:%d sourceChatId:%s sourceMessageId:%d sourceMediaKey:%s", row.Id, row.SourceChatId, row.SourceMessageId, row.SourceMediaKey)
		return true, nil
	}
	return false, nil
}

func (s *sSysPublish) cacheDownloadedCollectMediaGroupToBackup(ctx context.Context, event gdb.Record, items []collectMediaItem) ([]collectMediaItem, bool, error) {
	if len(items) == 0 {
		return items, false, nil
	}
	channels, err := s.collectEventBackupChannels(ctx, event)
	if err != nil {
		return items, false, err
	}
	if len(channels) == 0 {
		return items, false, nil
	}
	for _, backup := range rotateCollectBackupChannels(channels, event["id"].Int64()) {
		if backup == nil || strings.TrimSpace(backup.ChannelId) == "" {
			continue
		}
		botIds, err := s.collectBackupCopyValidationBotIds(ctx, event, backup.ChannelId)
		if err != nil {
			return items, false, err
		}
		if len(botIds) == 0 {
			continue
		}
		botToken, err := s.telegramJobBotToken(ctx, botIds[0], event["tenant_id"].Int64())
		if err != nil {
			return items, false, err
		}
		bot, err := s.telegramBot(ctx, botToken)
		if err != nil {
			return items, false, err
		}
		chatId := normalizeTelegramChannelChatID(backup.ChannelId)
		media := make([]*telegramMediaItem, 0, len(items))
		for index, item := range items {
			media = append(media, &telegramMediaItem{
				Id:          int64(index + 1),
				MediaType:   collectPublishMediaType(item.Type),
				FileUrl:     item.FileUrl,
				StoragePath: item.StoragePath,
				PosterUrl:   item.PosterUrl,
			})
		}
		messages, err := s.sendTelegramMediaSet(ctx, bot, chatId, "collect_backup", "", media)
		if err != nil {
			s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "下载媒体组推送备份频道失败，尝试下一个备份频道", backup.ChannelId+": "+err.Error())
			continue
		}
		if len(messages) != len(items) {
			s.cleanupTelegramSentMessages(ctx, bot, chatId, messages, "下载媒体组推送备份频道返回数量不完整")
			s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "下载媒体组推送备份频道返回数量不完整，尝试下一个备份频道", backup.ChannelId)
			continue
		}
		messageIds := make([]int, 0, len(messages))
		for _, message := range messages {
			if message == nil || message.MessageId <= 0 {
				continue
			}
			messageIds = append(messageIds, int(message.MessageId))
		}
		if len(messageIds) != len(items) {
			s.cleanupTelegramSentMessages(ctx, bot, chatId, messages, "下载媒体组推送备份频道缺少消息ID")
			s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "下载媒体组推送备份频道缺少消息ID，尝试下一个备份频道", backup.ChannelId)
			continue
		}
		if err = s.validateCollectBackupCopyRefs(ctx, event, backup.ChannelId, messageIds); err != nil {
			s.appendCollectEventLogForRecord(ctx, event, "media", "retry", "下载媒体组备份消息不可复制，尝试下一个备份频道", backup.ChannelId+": "+err.Error())
			continue
		}
		cached := make([]collectMediaItem, len(items))
		for index, item := range items {
			cached[index] = item
			cached[index].FileId = telegramCopyMediaFileId(backup.ChannelId, messageIds[index])
		}
		s.appendCollectEventLogForRecord(ctx, event, "media", "forwarded", "下载媒体组已推送到备份频道", fmt.Sprintf("channel=%s messages=%v", backup.ChannelId, messageIds))
		return cached, true, nil
	}
	return items, false, gerror.New("下载媒体组推送所有备份频道失败")
}

func collectMediaBackupChatId(item collectMediaItem) string {
	ref, ok := telegramCopyMediaRefFromFileId(item.FileId)
	if !ok {
		return ""
	}
	return ref.ChatId
}

func collectMediaBackupMessageId(item collectMediaItem) int {
	ref, ok := telegramCopyMediaRefFromFileId(item.FileId)
	if !ok {
		return 0
	}
	return ref.MessageId
}

func (s *sSysPublish) updateCollectEventMediaItems(ctx context.Context, rows []*entity.YoubanPublishCollectEventMedia, items []collectMediaItem) (bool, error) {
	changed := false
	cols := pdao.YoubanPublishCollectEventMedia.Columns()
	for index, row := range rows {
		if row == nil || index >= len(items) {
			continue
		}
		item := normalizeCollectMediaItem(items[index])
		data := g.Map{
			cols.SourceFileId:       item.FileId,
			cols.SourceRefType:      collectMediaRefType(item),
			cols.FileUrl:            item.FileUrl,
			cols.StoragePath:        item.StoragePath,
			cols.PosterUrl:          item.PosterUrl,
			cols.CacheStatus:        collectMediaCacheReady,
			cols.ErrorMessage:       "",
			collectMediaNextRetryAt: nil,
			cols.UpdatedAt:          gtime.Now(),
		}
		if strings.TrimSpace(item.MetaJson) != "" {
			data[cols.MetaJson] = item.MetaJson
		}
		if ref, ok := telegramCopyMediaRefFromFileId(item.FileId); ok {
			data[cols.BackupChatId] = ref.ChatId
			data[cols.BackupMessageId] = ref.MessageId
		}
		if _, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where(cols.Id, row.Id).Data(data).Update(); err != nil {
			return changed, gerror.Wrap(err, "更新采集媒体备份结果失败")
		}
		changed = true
	}
	return changed, nil
}

func (s *sSysPublish) collectEventNeedsMediaCache(ctx context.Context, event gdb.Record) bool {
	rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
	if err != nil {
		return false
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if collectMediaRowNeedsCache(row.SourceFileId, row.SourceMessageRef, row.StoragePath, row.FileUrl, row.BackupChatId, row.BackupMessageId) {
			return true
		}
	}
	return false
}

func collectMediaRowNeedsCache(sourceFileId string, sourceMessageRef string, storagePath string, fileUrl string, backupChatId string, backupMessageId int64) bool {
	sourceRef := strings.TrimSpace(sourceFileId)
	if sourceRef == "" {
		sourceRef = strings.TrimSpace(sourceMessageRef)
	}
	return strings.HasPrefix(sourceRef, "gotd:") &&
		strings.TrimSpace(storagePath) == "" &&
		strings.TrimSpace(fileUrl) == "" &&
		!(strings.TrimSpace(backupChatId) != "" && backupMessageId > 0)
}

func collectEventMediaCacheView(mediaJSON string, mediaCount int, status string, errorMessage string) (string, string) {
	if mediaCount <= 0 || strings.TrimSpace(mediaJSON) == "" || strings.TrimSpace(mediaJSON) == "[]" {
		return "none", "无媒体"
	}
	if collectMediaErrorIsRateLimited(errorMessage) {
		return "rate_limited", errorMessage
	}
	var items []collectMediaItem
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		return "failed", "媒体数据异常"
	}
	pending := 0
	cached := 0
	for _, item := range items {
		if strings.HasPrefix(strings.TrimSpace(item.FileId), "gotd:") &&
			strings.TrimSpace(item.StoragePath) == "" &&
			strings.TrimSpace(item.FileUrl) == "" {
			pending++
			continue
		}
		if strings.TrimSpace(item.StoragePath) != "" || strings.TrimSpace(item.FileUrl) != "" || strings.TrimSpace(item.FileId) != "" {
			cached++
		}
	}
	if pending > 0 {
		if strings.TrimSpace(status) == sysin.CollectEventStatusFailed && strings.TrimSpace(errorMessage) != "" {
			return "failed", errorMessage
		}
		return "caching", fmt.Sprintf("%d 个媒体等待缓存", pending)
	}
	if cached > 0 {
		return "cached", fmt.Sprintf("%d 个媒体已缓存", cached)
	}
	return "none", "无可缓存媒体"
}

func collectMediaErrorIsRateLimited(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	return strings.Contains(message, "限流") ||
		strings.Contains(message, "too many requests") ||
		strings.Contains(message, "flood_wait")
}

func collectMediaAuthBytesInvalid(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "auth_bytes_invalid")
}

func (s *sSysPublish) downloadTelegramMedia(ctx context.Context, tenantId int64, tgAccountId int64, item collectMediaItem) (*collectDownloadedMedia, error) {
	startedAt := time.Now()
	if tgAccountId <= 0 {
		return nil, gerror.New("账号采集媒体缺少TG账号")
	}
	var meta gotdCollectMediaMeta
	if err := json.Unmarshal([]byte(item.MetaJson), &meta); err != nil || meta.Id <= 0 {
		return nil, gerror.New("账号采集媒体缺少下载元数据")
	}
	const downloadTimeout = 10 * time.Minute
	downloadCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	clientStartedAt := time.Now()
	client, err := s.accountCollectRuntimeClient(tgAccountId)
	if err != nil {
		g.Log().Warningf(ctx, "TG媒体下载获取账号客户端失败 tgAccountId:%d fileId:%s lookupDuration:%s total:%s err:%+v", tgAccountId, item.FileId, time.Since(clientStartedAt).Round(time.Millisecond), time.Since(startedAt).Round(time.Millisecond), err)
		var retryErr *collectMediaRetryError
		if errors.As(err, &retryErr) {
			return nil, retryErr
		}
		return nil, newCollectMediaRetryError("账号采集客户端暂不可用，等待重试: "+err.Error(), 30*time.Second)
	}
	g.Log().Debugf(ctx, "TG媒体下载获取账号客户端完成 tgAccountId:%d fileId:%s duration:%s", tgAccountId, item.FileId, time.Since(clientStartedAt).Round(time.Millisecond))
	transferStartedAt := time.Now()
	downloadItem := item
	result, err := s.downloadTelegramMediaWithClient(downloadCtx, tenantId, tgAccountId, downloadItem, meta, client)
	if collectMediaFileReferenceExpired(err) {
		refreshStartedAt := time.Now()
		refreshedItem, refreshErr := s.refreshGotdCollectMediaItem(downloadCtx, tenantId, tgAccountId, client, item)
		if refreshErr != nil {
			g.Log().Warningf(ctx, "TG媒体文件引用刷新失败 tgAccountId:%d fileId:%s duration:%s err:%+v", tgAccountId, item.FileId, time.Since(refreshStartedAt).Round(time.Millisecond), refreshErr)
		} else {
			downloadItem = refreshedItem
			meta, _ = gotdCollectMediaMetaFromItem(downloadItem)
			g.Log().Infof(ctx, "TG媒体文件引用已刷新，重试下载 tgAccountId:%d fileId:%s duration:%s", tgAccountId, item.FileId, time.Since(refreshStartedAt).Round(time.Millisecond))
			result, err = s.downloadTelegramMediaWithClient(downloadCtx, tenantId, tgAccountId, downloadItem, meta, client)
		}
	}
	if err != nil {
		g.Log().Warningf(ctx, "TG媒体下载传输失败 tgAccountId:%d fileId:%s size:%d dc:%d duration:%s total:%s err:%+v", tgAccountId, item.FileId, meta.Size, meta.DCID, time.Since(transferStartedAt).Round(time.Millisecond), time.Since(startedAt).Round(time.Millisecond), err)
		if collectMediaShouldReconnectAccount(err) {
			delay := s.openAccountCollectCircuit(ctx, tgAccountId, err)
			s.restartAccountCollectWorker(ctx, tgAccountId, err)
			if retryErr := collectMediaRetryErrorFrom(err); retryErr != nil {
				if delay > retryErr.delay {
					retryErr.delay = delay
				}
				retryErr.message = fmt.Sprintf("TG账号连接异常，账号级熔断等待%s后自动恢复：%v", retryErr.delay.Round(time.Second), err)
				return nil, retryErr
			}
		}
		return nil, err
	}
	if result == nil || strings.TrimSpace(result.Path) == "" {
		return nil, gerror.New("账号采集媒体下载完成但未返回缓存文件")
	}
	g.Log().Infof(ctx, "TG媒体下载链路完成 tgAccountId:%d fileId:%s size:%d dc:%d transfer:%s total:%s path:%s", tgAccountId, item.FileId, meta.Size, meta.DCID, time.Since(transferStartedAt).Round(time.Millisecond), time.Since(startedAt).Round(time.Millisecond), result.Path)
	return result, nil
}

func (s *sSysPublish) refreshGotdCollectMediaItem(ctx context.Context, tenantId int64, tgAccountId int64, client *telegram.Client, item collectMediaItem) (collectMediaItem, error) {
	chatId, messageId, ok := parseGotdCollectFileId(item.FileId)
	if !ok {
		return item, gerror.New("TG媒体缺少可刷新的原消息引用")
	}
	peer, err := s.collectMediaInputPeer(ctx, tenantId, tgAccountId, client, chatId)
	if err != nil {
		return item, err
	}
	var result tg.MessagesMessagesClass
	if channelPeer, ok := peer.(*tg.InputPeerChannel); ok {
		result, err = client.API().ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: channelPeer.ChannelID, AccessHash: channelPeer.AccessHash},
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: messageId}},
		})
	} else {
		result, err = client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:      peer,
			OffsetID:  messageId + 1,
			AddOffset: -1,
			Limit:     1,
			MaxID:     messageId,
			MinID:     messageId,
		})
	}
	if err != nil {
		return item, gerror.Wrap(err, "重新读取TG原消息失败")
	}
	for _, message := range tgHistoryMessages(result) {
		if message == nil || message.ID != messageId {
			continue
		}
		refreshed := gotdCollectMedia(message, chatId)
		if len(refreshed) == 0 {
			return item, gerror.New("TG原消息已不存在媒体")
		}
		refreshed[0].FileId = item.FileId
		refreshed[0].Purpose = item.Purpose
		return refreshed[0], nil
	}
	return item, gerror.New("TG原消息不存在或无权读取")
}

func (s *sSysPublish) downloadTelegramMediaWithClient(ctx context.Context, _ int64, tgAccountId int64, item collectMediaItem, meta gotdCollectMediaMeta, client *telegram.Client) (*collectDownloadedMedia, error) {
	startedAt := time.Now()
	if client == nil {
		return nil, gerror.New("账号采集媒体下载客户端为空")
	}
	key := mediaFileCacheKey(&telegramMediaItem{
		MediaType:   listenerTelegramMediaType(item.Type),
		StoragePath: item.FileId,
		AssetHash:   gotdMediaCacheAssetKey(meta),
	}, collectTelegramMediaCacheSource(item, tgAccountId))
	dir := mediaFileCacheDir(ctx)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, gerror.Wrap(err, "创建媒体缓存目录失败")
	}
	filePath := mediaFileCachePath(dir, key, collectMediaExt(item.Type, meta.MimeType))
	metaPath := filePath + ".json"
	if fileExists(filePath) {
		if err := touchMediaFileCacheMeta(metaPath, key, item.FileId, filePath); err != nil {
			g.Log().Warningf(ctx, "更新TG媒体缓存访问时间失败 path:%s err:%+v", filePath, err)
		}
		g.Log().Debugf(ctx, "TG媒体下载命中本地缓存 tgAccountId:%d fileId:%s size:%d duration:%s path:%s", tgAccountId, item.FileId, meta.Size, time.Since(startedAt).Round(time.Millisecond), filePath)
		return &collectDownloadedMedia{Path: filePath}, nil
	}
	g.Log().Infof(ctx, "TG媒体下载进入传输 tgAccountId:%d fileId:%s size:%d dc:%d path:%s", tgAccountId, item.FileId, meta.Size, meta.DCID, filePath)
	transferStartedAt := time.Now()
	value, err, _ := mediaFileCacheDownloadGroup.Do("gotd:"+key, func() (interface{}, error) {
		if fileExists(filePath) {
			if err := touchMediaFileCacheMeta(metaPath, key, item.FileId, filePath); err != nil {
				g.Log().Warningf(ctx, "更新TG媒体缓存访问时间失败 path:%s err:%+v", filePath, err)
			}
			return collectMediaCacheResult{path: filePath, metaJSON: item.MetaJson}, nil
		}
		cachedMeta, err := s.downloadGotdCollectMediaToFile(ctx, tgAccountId, item, meta, client, filePath)
		if err != nil {
			return "", err
		}
		if err := touchMediaFileCacheMeta(metaPath, key, item.FileId, filePath); err != nil {
			return "", err
		}
		if err := pruneMediaFileCache(ctx); err != nil {
			g.Log().Warningf(ctx, "清理媒体文件缓存失败: %+v", err)
		}
		metaJSON, marshalErr := json.Marshal(cachedMeta)
		if marshalErr != nil {
			return "", gerror.Wrap(marshalErr, "序列化TG媒体最新引用失败")
		}
		return collectMediaCacheResult{path: filePath, metaJSON: string(metaJSON)}, nil
	})
	if err != nil {
		g.Log().Warningf(ctx, "TG媒体下载 singleflight 返回错误 tgAccountId:%d fileId:%s duration:%s total:%s err:%+v", tgAccountId, item.FileId, time.Since(transferStartedAt).Round(time.Millisecond), time.Since(startedAt).Round(time.Millisecond), err)
		return nil, err
	}
	result, ok := value.(collectMediaCacheResult)
	if !ok || strings.TrimSpace(result.path) == "" {
		return nil, gerror.New("TG媒体缓存下载返回空路径")
	}
	g.Log().Infof(ctx, "TG媒体下载 singleflight 完成 tgAccountId:%d fileId:%s duration:%s total:%s path:%s", tgAccountId, item.FileId, time.Since(transferStartedAt).Round(time.Millisecond), time.Since(startedAt).Round(time.Millisecond), result.path)
	return &collectDownloadedMedia{Path: result.path, MetaJson: result.metaJSON}, nil
}

func (s *sSysPublish) downloadGotdCollectMediaToFile(ctx context.Context, tgAccountId int64, item collectMediaItem, meta gotdCollectMediaMeta, client *telegram.Client, filePath string) (gotdCollectMediaMeta, error) {
	startedAt := time.Now()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return meta, gerror.Wrap(err, "创建TG媒体缓存子目录失败")
	}
	partPath := filePath + ".part"
	_ = os.Remove(partPath)
	threads := g.Cfg().MustGet(ctx, "youbanPublish.collect.mediaDownloadThreads", 2).Int()
	if threads < 1 {
		threads = 1
	}
	if threads > 8 {
		threads = 8
	}
	g.Log().Infof(ctx, "TG媒体文件传输开始 tgAccountId:%d fileId:%s size:%d dc:%d threads:%d partSize:%d path:%s", tgAccountId, item.FileId, meta.Size, meta.DCID, threads, tgMediaTransferPartSize, filePath)
	err := globalTGMediaTransferManager.download(
		ctx,
		tgAccountId,
		client,
		gotdInputFileLocation(meta),
		partPath,
		meta.Size,
		meta.DCID,
		threads,
	)
	if err != nil {
		_ = os.Remove(partPath)
		g.Log().Warningf(ctx, "TG媒体文件传输失败 tgAccountId:%d fileId:%s size:%d dc:%d threads:%d duration:%s err:%+v", tgAccountId, item.FileId, meta.Size, meta.DCID, threads, time.Since(startedAt).Round(time.Millisecond), err)
		return meta, gerror.Wrap(err, "下载TG媒体到缓存失败")
	}
	if err = os.Rename(partPath, filePath); err != nil {
		_ = os.Remove(partPath)
		return meta, gerror.Wrap(err, "写入TG媒体缓存文件失败")
	}
	g.Log().Infof(ctx, "TG媒体文件传输完成 tgAccountId:%d fileId:%s size:%d dc:%d threads:%d duration:%s path:%s", tgAccountId, item.FileId, meta.Size, meta.DCID, threads, time.Since(startedAt).Round(time.Millisecond), filePath)
	return meta, nil
}

func collectTelegramMediaCacheSource(item collectMediaItem, tgAccountId int64) string {
	fileID := strings.TrimSpace(item.FileId)
	if strings.HasPrefix(fileID, "gotd:") {
		return "gotd:global:" + fileID
	}
	return fmt.Sprintf("gotd:account:%d:%s", tgAccountId, fileID)
}

func gotdMediaCacheAssetKey(meta gotdCollectMediaMeta) string {
	return fmt.Sprintf("%s:%d:%d:%s:%s:%d:%d", meta.Kind, meta.Id, meta.AccessHash, meta.ThumbSize, meta.MimeType, meta.DCID, meta.Size)
}

func collectMediaFileReferenceExpired(err error) bool {
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "FILE_REFERENCE_EXPIRED")
}

func (s *sSysPublish) discardCollectEventGroup(ctx context.Context, eventId int64, reason string) error {
	if eventId <= 0 {
		return nil
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, table := range []string{
			pdao.YoubanPublishCollectEventMedia.Table(),
			pdao.YoubanPublishCollectEventLog.Table(),
			pdao.YoubanPublishCollectReview.Table(),
			pdao.YoubanPublishCollectDispatch.Table(),
			"hg_youban_publish_collect_media_stat",
		} {
			if _, err := tx.Model(table).Where("event_id", eventId).Delete(); err != nil {
				return gerror.Wrapf(err, "丢弃采集资料组失败 table:%s", table)
			}
		}
		if _, err := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Where("id", eventId).Delete(); err != nil {
			return gerror.Wrap(err, "丢弃采集资料组事件失败")
		}
		g.Log().Warningf(ctx, "采集资料组已丢弃 eventId:%d reason:%s", eventId, reason)
		return nil
	})
}

func (s *sSysPublish) collectMediaInputPeer(ctx context.Context, tenantId int64, tgAccountId int64, client *telegram.Client, chatId string) (tg.InputPeerClass, error) {
	if client == nil {
		return nil, gerror.New("账号采集媒体下载客户端为空")
	}
	cache, err := s.tgChannelCacheByChannelId(ctx, tenantId, tgAccountId, chatId)
	if err == nil && cache != nil {
		if peer, peerErr := collectInputPeerChannel(cache); peerErr == nil {
			return peer, nil
		}
	}
	id, parseErr := strconv.ParseInt(strings.TrimSpace(chatId), 10, 64)
	if parseErr == nil && id > 0 {
		return &tg.InputPeerUser{UserID: id}, nil
	}
	return nil, gerror.New("账号采集媒体无法解析原消息会话")
}

func gotdInputFileLocation(meta gotdCollectMediaMeta) tg.InputFileLocationClass {
	if meta.Kind == "photo" {
		thumbSize := strings.TrimSpace(meta.ThumbSize)
		if thumbSize == "" {
			thumbSize = "y"
		}
		return &tg.InputPhotoFileLocation{
			ID:            meta.Id,
			AccessHash:    meta.AccessHash,
			FileReference: meta.FileReference,
			ThumbSize:     thumbSize,
		}
	}
	return &tg.InputDocumentFileLocation{
		ID:            meta.Id,
		AccessHash:    meta.AccessHash,
		FileReference: meta.FileReference,
	}
}

func collectMediaUploadContext(ctx context.Context, accountId int64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	current := contexts.Get(ctx)
	if current == nil {
		return context.WithValue(ctx, consts.ContextHTTPKey, &model.Context{
			Module:    consts.AppApi,
			AddonName: "youban_publish",
			User:      &model.Identity{Id: accountId, App: consts.AppApi},
			Data:      g.Map{},
		})
	}
	if current.Module == "" {
		current.Module = consts.AppApi
	}
	if current.AddonName == "" {
		current.AddonName = "youban_publish"
	}
	if current.User == nil || current.User.Id <= 0 {
		current.User = &model.Identity{Id: accountId, App: consts.AppApi}
	}
	if current.User.App == "" {
		current.User.App = consts.AppApi
	}
	return ctx
}

func collectMediaExt(mediaType string, mimeType string) string {
	if strings.TrimSpace(mediaType) == "video" {
		return ".mp4"
	}
	if ext, _ := mime.ExtensionsByType(strings.TrimSpace(mimeType)); len(ext) > 0 {
		return ext[0]
	}
	return ".jpg"
}

type collectDownloadedMedia struct {
	AttachmentId int64
	FileUrl      string
	Path         string
	MetaJson     string
}

type collectMediaCacheResult struct {
	path     string
	metaJSON string
}

func (s *sSysPublish) acquireCollectMediaDownloadSlots(ctx context.Context, tenantId int64, tgAccountId int64) (func(), error) {
	if tenantId <= 0 || tgAccountId <= 0 {
		return func() {}, gerror.New("采集媒体下载缺少租户或TG账号")
	}
	accountKey := collectMediaAccountKey(tenantId, tgAccountId)
	globalLimit := g.Cfg().MustGet(ctx, "youbanPublish.collect.globalMediaConcurrency", 16).Int()
	if globalLimit < 1 {
		globalLimit = 1
	}
	if globalLimit > 64 {
		globalLimit = 64
	}
	accountLimit := g.Cfg().MustGet(ctx, "youbanPublish.collect.accountMediaConcurrency", 2).Int()
	if accountLimit < 1 {
		accountLimit = 1
	}
	if accountLimit > 8 {
		accountLimit = 8
	}
	s.collectMediaMu.Lock()
	if s.collectMediaAccounts == nil {
		s.collectMediaAccounts = make(map[string]chan struct{})
	}
	if s.collectMediaSlots == nil || cap(s.collectMediaSlots) != globalLimit {
		s.collectMediaSlots = make(chan struct{}, globalLimit)
	}
	accountSlots := s.collectMediaAccounts[accountKey]
	if accountSlots == nil || cap(accountSlots) != accountLimit {
		accountSlots = make(chan struct{}, accountLimit)
		s.collectMediaAccounts[accountKey] = accountSlots
	}
	globalSlots := s.collectMediaSlots
	s.collectMediaMu.Unlock()
	select {
	case accountSlots <- struct{}{}:
	case <-ctx.Done():
		return func() {}, ctx.Err()
	}
	select {
	case globalSlots <- struct{}{}:
		return func() {
			<-globalSlots
			<-accountSlots
		}, nil
	case <-ctx.Done():
		<-accountSlots
		return func() {}, ctx.Err()
	}
}

func (s *sSysPublish) waitCollectMediaAccountInterval(ctx context.Context, tenantId int64, tgAccountId int64) {
	interval := time.Duration(g.Cfg().MustGet(ctx, "youbanPublish.collect.mediaAccountIntervalMs", 500).Int()) * time.Millisecond
	if interval <= 0 {
		return
	}
	key := collectMediaAccountKey(tenantId, tgAccountId)
	s.collectMediaMu.Lock()
	last := s.collectMediaLastTouch[key]
	wait := interval - time.Since(last)
	s.collectMediaLastTouch[key] = time.Now().Add(maxDuration(wait, 0))
	s.collectMediaMu.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
	}
}

func collectMediaAccountKey(tenantId int64, tgAccountId int64) string {
	return fmt.Sprintf("%d:%d", tenantId, tgAccountId)
}

func maxDuration(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
