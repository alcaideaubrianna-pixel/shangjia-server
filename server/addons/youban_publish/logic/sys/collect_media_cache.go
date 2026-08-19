package sys

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	pdao "hotgo/addons/youban_publish/internal/dao"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/hgrds/lock"
	"hotgo/internal/model"
)

type collectMediaRetryError struct {
	message             string
	delay               time.Duration
	rateLimited         bool
	deferWithoutFailure bool
}

type collectEventMediaCacheSummary struct {
	Total       int
	Ready       int
	Pending     int
	Downloading int
	Failed      int
}

type collectMediaDiscardedError struct {
	message string
}

func (e *collectMediaDiscardedError) Error() string {
	return e.message
}

const defaultCollectMediaDownloadThreads = 4

var errCollectMediaSourceGone = errors.New("TG原消息已删除或已无可用媒体")

func (e *collectMediaRetryError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func (e *collectMediaRetryError) AccountTaskRetryDelay() time.Duration {
	if e == nil {
		return 0
	}
	return e.delay
}

func newCollectMediaRetryError(message string, delay time.Duration) *collectMediaRetryError {
	if delay <= 0 {
		delay = 15 * time.Second
	}
	return &collectMediaRetryError{message: message, delay: delay}
}

func newCollectMediaFairnessRetryError(message string, delay time.Duration) *collectMediaRetryError {
	retryErr := newCollectMediaRetryError(message, delay)
	retryErr.deferWithoutFailure = true
	return retryErr
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
	enabled, err := collectProcessSourceEnabled(ctx, collectProcessQueuePayload{
		TenantId:  payload.TenantId,
		AccountId: payload.AccountId,
		SourceId:  payload.SourceId,
	})
	if err != nil {
		return gerror.Wrap(err, "检查采集源媒体任务状态失败")
	}
	if !enabled {
		return nil
	}
	taskStartedAt := time.Now()
	var statEvent gdb.Record
	defer func() {
		if !statEvent.IsEmpty() {
			s.recordCollectMediaPerformance(context.Background(), statEvent, taskStartedAt, retErr)
		}
	}()
	ctx = collectMediaRuntimeContext(ctx, payload.AccountId)
	g.Log().Debugf(ctx, "采集媒体缓存任务开始 eventId:%d tenantId:%d accountId:%d sourceId:%d tgAccountId:%d", payload.EventId, payload.TenantId, payload.AccountId, payload.SourceId, payload.TgAccountId)
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
	g.Log().Debugf(ctx, "采集媒体任务获取事件锁完成 eventId:%d wait:%s", payload.EventId, time.Since(lockStartedAt).Round(time.Millisecond))
	defer func() { _ = distributedLock.Unlock(context.Background()) }()
	accountIntervalStartedAt := time.Now()
	s.waitCollectMediaAccountInterval(ctx, payload.TenantId, payload.TgAccountId)
	g.Log().Debugf(ctx, "采集媒体任务账号间隔等待完成 eventId:%d wait:%s", payload.EventId, time.Since(accountIntervalStartedAt).Round(time.Millisecond))
	readEventStartedAt := time.Now()
	event, err := s.collectMediaCacheEvent(ctx, payload)
	if err != nil {
		return err
	}
	g.Log().Debugf(ctx, "采集媒体任务读取事件完成 eventId:%d duration:%s status:%s", payload.EventId, time.Since(readEventStartedAt).Round(time.Millisecond), event["status"].String())
	if event.IsEmpty() {
		g.Log().Warningf(ctx, "采集媒体任务对应事件不存在，跳过历史任务 eventId:%d tenantId:%d accountId:%d sourceId:%d", payload.EventId, payload.TenantId, payload.AccountId, payload.SourceId)
		return nil
	}
	if event["source_type"].String() == sysin.CollectSourceTypeBot && event["bot_id"].Int64() <= 0 {
		botID, resolveErr := s.resolveCollectEventBotID(ctx, event)
		if resolveErr != nil {
			return resolveErr
		}
		event["bot_id"] = g.NewVar(botID)
	}
	statEvent = event
	if collectEventAlreadyMatched(event["status"].String()) {
		g.Log().Debugf(ctx, "采集媒体任务对应事件已完成，跳过重复任务 eventId:%d status:%s", payload.EventId, event["status"].String())
		return nil
	}
	if event["status"].String() == sysin.CollectEventStatusIgnored {
		g.Log().Debugf(ctx, "采集媒体任务对应事件已忽略，取消媒体下载 eventId:%d", payload.EventId)
		return nil
	}
	if event["material_role"].String() == collectMaterialRoleVerify && event["material_parent_event_id"].Int64() > 0 {
		parent, parentErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Fields("status,error_message").
			Where("id", event["material_parent_event_id"].Int64()).
			One()
		if parentErr != nil {
			return gerror.Wrap(parentErr, "检查验证媒体父展示事件失败")
		}
		if !parent.IsEmpty() && (parent["status"].String() == sysin.CollectEventStatusIgnored || parent["status"].String() == sysin.CollectEventStatusFailed) {
			message := strings.TrimSpace(parent["error_message"].String())
			if message == "" {
				message = "父展示资料已忽略"
			}
			g.Log().Infof(ctx, "验证媒体父展示资料已忽略，取消媒体下载 eventId:%d parentEventId:%d", payload.EventId, event["material_parent_event_id"].Int64())
			return s.ignoreCollectEvent(ctx, payload.EventId, message, "group")
		}
		if !parent.IsEmpty() && !collectDisplayEventPassedEarlyCheck(parent["status"].String()) {
			g.Log().Infof(
				ctx,
				"验证媒体等待父展示资料预检，取消旧媒体任务 eventId:%d parentEventId:%d parentStatus:%s",
				payload.EventId,
				event["material_parent_event_id"].Int64(),
				parent["status"].String(),
			)
			return s.markCollectEvent(
				ctx,
				payload.EventId,
				sysin.CollectEventStatusGroupCollect,
				"等待父展示资料完成规则和去重预检",
			)
		}
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
	if err != nil {
		g.Log().Warningf(ctx, "采集媒体任务媒体阶段未完成 eventId:%d duration:%s changed:%t err:%v", payload.EventId, time.Since(cacheStartedAt).Round(time.Millisecond), changed, err)
	} else {
		g.Log().Debugf(ctx, "采集媒体任务媒体阶段完成 eventId:%d duration:%s changed:%t", payload.EventId, time.Since(cacheStartedAt).Round(time.Millisecond), changed)
	}
	if err != nil {
		var discardedErr *collectMediaDiscardedError
		if errors.As(err, &discardedErr) {
			g.Log().Infof(ctx, "采集媒体来源消息已删除，已丢弃整组资料 eventId:%d reason:%s", payload.EventId, discardedErr.message)
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
	g.Log().Debugf(ctx, "采集媒体缓存任务完成 eventId:%d changed:%t total:%s", payload.EventId, changed, time.Since(taskStartedAt).Round(time.Millisecond))
	s.appendCollectEventLogForRecord(ctx, event, "media", "ready", "媒体缓存任务处理完成", "")
	if err := s.processCollectEvent(ctx, payload.EventId, payload.TenantId, payload.AccountId); err != nil {
		if isCollectProcessRetryError(err) {
			g.Log().Infof(ctx, "媒体缓存完成后等待采集事件依赖 eventId:%d err:%s", payload.EventId, err.Error())
			return nil
		}
		g.Log().Errorf(ctx, "媒体缓存完成后继续处理采集事件失败 eventId:%d err:%+v", payload.EventId, err)
		return err
	}
	return nil
}

func (s *sSysPublish) resolveCollectEventBotID(ctx context.Context, event gdb.Record) (int64, error) {
	if event.IsEmpty() {
		return 0, gerror.New("Bot采集事件为空")
	}
	botID := event["bot_id"].Int64()
	if botID > 0 {
		return botID, nil
	}
	source, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields("bot_id").
		Where("id", event["source_id"].Int64()).
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		Where("source_type", sysin.CollectSourceTypeBot).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取Bot采集源媒体配置失败")
	}
	botID = source["bot_id"].Int64()
	if botID <= 0 {
		return 0, gerror.New("Bot采集媒体缺少Bot ID")
	}
	_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", event["id"].Int64()).
		WhereLTE("bot_id", 0).
		Data(g.Map{"bot_id": botID, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return 0, gerror.Wrap(err, "补全Bot采集事件媒体配置失败")
	}
	g.Log().Infof(ctx, "已从采集源补全Bot采集事件媒体配置 eventId:%d sourceId:%d botId:%d", event["id"].Int64(), event["source_id"].Int64(), botID)
	return botID, nil
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
	g.Log().Debugf(ctx, "采集媒体阶段读取媒体完成 eventId:%d sourceType:%s mediaCount:%d duration:%s", event["id"].Int64(), event["source_type"].String(), len(rows), time.Since(stageStartedAt).Round(time.Millisecond))
	s.appendCollectEventLogForRecord(ctx, event, "media", "checking", "开始检查媒体缓存方式", fmt.Sprintf("media=%d", len(items)))
	changed := false
	s.appendCollectEventLogForRecord(ctx, event, "media", "downloading", "采集媒体使用统一下载与云存储缓存", "")
	type downloadResult struct {
		index int
		row   *collectEventMediaRow
		item  collectMediaItem
		err   error
	}
	results := make([]downloadResult, len(rows))
	fileSlots := make(chan struct{}, accountCollectMediaConcurrency(ctx))
	var downloadWait sync.WaitGroup
	for index, row := range rows {
		if row == nil || !collectEventMediaRowNeedsCache(event, row) {
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
		go func(index int, row *collectEventMediaRow) {
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
			releaseAccountSlot := func() {}
			var slotErr error
			if event["source_type"].String() != sysin.CollectSourceTypeBot {
				releaseAccountSlot, slotErr = s.acquireCollectMediaDownloadSlots(ctx, event["tenant_id"].Int64(), event["tg_account_id"].Int64())
			}
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
			sourceType := event["source_type"].String()
			logMessage := "开始下载账号采集媒体"
			if sourceType == sysin.CollectSourceTypeBot {
				logMessage = "开始下载Bot采集媒体"
			}
			s.appendCollectEventLogForRecord(ctx, event, "media", "downloading", logMessage, fmt.Sprintf("mediaId=%d sourceMessageId=%d sourceFileId=%s", row.Id, row.SourceMessageId, row.SourceFileId))
			var cached *collectDownloadedMedia
			if sourceType == sysin.CollectSourceTypeBot {
				cached, err = s.downloadBotTelegramMedia(ctx, event["tenant_id"].Int64(), event["bot_id"].Int64(), items[index])
			} else {
				cached, err = s.downloadTelegramMedia(ctx, event["tenant_id"].Int64(), event["account_id"].Int64(), event["tg_account_id"].Int64(), items[index])
			}
			if err != nil {
				downloadDuration := time.Since(startedAt).Milliseconds()
				errorType := collectMediaErrorType(err.Error())
				retryErr := collectMediaRetryErrorFrom(err)
				if retryErr != nil {
					g.Log().Warningf(ctx, "采集媒体下载暂不可用，等待自动重试 eventId:%d mediaId:%d sourceMessageId:%d duration:%s err:%+v", event["id"].Int64(), row.Id, row.SourceMessageId, time.Since(startedAt).Round(time.Millisecond), err)
				} else {
					g.Log().Errorf(ctx, "采集媒体下载失败 eventId:%d mediaId:%d sourceMessageId:%d duration:%s err:%+v", event["id"].Int64(), row.Id, row.SourceMessageId, time.Since(startedAt).Round(time.Millisecond), err)
				}
				if collectMediaAuthBytesInvalid(err) {
					if sourceType != sysin.CollectSourceTypeBot {
						s.restartAccountCollectWorker(ctx, event["tg_account_id"].Int64(), err)
					}
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
				g.Log().Errorf(ctx, "采集媒体下载返回空结果 eventId:%d mediaId:%d sourceMessageId:%d duration:%s", event["id"].Int64(), row.Id, row.SourceMessageId, time.Since(startedAt).Round(time.Millisecond))
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
			g.Log().Debugf(ctx, "采集媒体下载完成 eventId:%d mediaId:%d sourceMessageId:%d duration:%s size:%d sourceType:%s", event["id"].Int64(), row.Id, row.SourceMessageId, time.Since(startedAt).Round(time.Millisecond), cachedSize, sourceType)
			result.item = cached.Item
			if strings.TrimSpace(result.item.FileId) == "" {
				result.item = items[index]
			}
			result.item.FileUrl = cached.FileUrl
			result.item.StoragePath = cached.Path
			if strings.TrimSpace(result.item.PosterUrl) == "" {
				result.item.PosterUrl = items[index].PosterUrl
			}
			result.item.DebugMetaJson = strings.TrimSpace(cached.MetaJson)
			if result.item.DebugMetaJson == "" {
				result.item.DebugMetaJson = items[index].DebugMetaJson
			}
			_, updateErr := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where("id", row.Id).Data(g.Map{
				"source_file_id":        result.item.FileId,
				"source_ref_type":       collectMediaRefType(result.item),
				"source_kind":           result.item.SourceKind,
				"source_media_id":       result.item.SourceMediaId,
				"source_access_hash":    result.item.SourceAccessHash,
				"source_file_reference": result.item.SourceFileReference,
				"source_thumb_size":     result.item.SourceThumbSize,
				"source_mime_type":      result.item.SourceMimeType,
				"source_dc_id":          result.item.SourceDCId,
				"source_size":           result.item.SourceSize,
				"file_url":              result.item.FileUrl,
				"storage_path":          result.item.StoragePath,
				"backup_chat_id":        "",
				"backup_message_id":     0,
				"cache_status":          collectMediaCacheReady,
				"download_duration_ms":  downloadDuration,
				"download_bytes":        cachedSize,
				"download_error_type":   "",
				"error_message":         "",
				collectMediaNextRetryAt: nil,
				"updated_at":            gtime.Now(),
			}).Update()
			if updateErr != nil {
				result.err = gerror.Wrap(updateErr, "更新采集媒体下载结果失败")
				results[index] = result
				return
			}
			g.Log().Debugf(ctx, "采集媒体下载结果已写回 eventId:%d mediaId:%d duration:%s size:%d", event["id"].Int64(), row.Id, time.Since(startedAt).Round(time.Millisecond), cachedSize)
			results[index] = result
		}(index, row)
	}
	downloadWait.Wait()
	for _, result := range results {
		if result.row != nil && result.err != nil && collectMediaSourceGone(result.err) {
			reason := "TG原消息已删除或无可用媒体，整组资料已丢弃"
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
		changed = true
	}
	return changed, nil
}

func (s *sSysPublish) reuseCollectMediaCache(ctx context.Context, row *collectEventMediaRow) (bool, error) {
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

func (s *sSysPublish) updateCollectEventMediaItems(ctx context.Context, rows []*collectEventMediaRow, items []collectMediaItem) (bool, error) {
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
			"source_kind":           item.SourceKind,
			"source_media_id":       item.SourceMediaId,
			"source_access_hash":    item.SourceAccessHash,
			"source_file_reference": item.SourceFileReference,
			"source_thumb_size":     item.SourceThumbSize,
			"source_mime_type":      item.SourceMimeType,
			"source_dc_id":          item.SourceDCId,
			"source_size":           item.SourceSize,
			"file_md5":              item.FileMd5,
			"file_phash":            item.FilePhash,
			cols.CacheStatus:        collectMediaCacheReady,
			cols.ErrorMessage:       "",
			collectMediaNextRetryAt: nil,
			cols.UpdatedAt:          gtime.Now(),
		}
		if strings.TrimSpace(item.DebugMetaJson) != "" {
			data[cols.MetaJson] = item.DebugMetaJson
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
		if collectEventMediaRowNeedsCache(event, row) {
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
	if sourceRef == "" || strings.TrimSpace(fileUrl) != "" ||
		(strings.TrimSpace(backupChatId) != "" && backupMessageId > 0) {
		return false
	}
	storagePath = strings.TrimSpace(storagePath)
	if storagePath == "" {
		return strings.HasPrefix(sourceRef, "gotd:")
	}
	if isCollectMediaCachePath(storagePath) {
		return !fileExists(resolveTelegramLocalPath(storagePath))
	}
	if filepath.IsAbs(storagePath) || strings.HasPrefix(storagePath, "attachment/") || strings.HasPrefix(storagePath, "upload/") || strings.HasPrefix(storagePath, "uploads/") {
		return !fileExists(resolveTelegramLocalPath(storagePath))
	}
	return false
}

func collectEventMediaRowNeedsCache(event gdb.Record, row *collectEventMediaRow) bool {
	if row == nil {
		return false
	}
	if collectMediaRowNeedsCache(row.SourceFileId, row.SourceMessageRef, row.StoragePath, row.FileUrl, row.BackupChatId, row.BackupMessageId) {
		return true
	}
	if event["source_type"].String() != sysin.CollectSourceTypeBot {
		return false
	}
	sourceFileId := strings.TrimSpace(row.SourceFileId)
	if sourceFileId == "" {
		sourceFileId = strings.TrimSpace(row.SourceMessageRef)
	}
	return sourceFileId != "" &&
		strings.TrimSpace(row.StoragePath) == "" &&
		strings.TrimSpace(row.FileUrl) == "" &&
		!(strings.TrimSpace(row.BackupChatId) != "" && row.BackupMessageId > 0)
}

func collectEventMediaCacheView(summary collectEventMediaCacheSummary, status string, errorMessage string) (string, string) {
	if summary.Total <= 0 {
		return "none", "无媒体"
	}
	if collectMediaErrorIsRateLimited(errorMessage) {
		return "rate_limited", errorMessage
	}
	if summary.Failed > 0 && strings.TrimSpace(errorMessage) != "" {
		return "failed", errorMessage
	}
	if summary.Downloading > 0 {
		return "caching", fmt.Sprintf("%d 个媒体下载中", summary.Downloading)
	}
	if summary.Pending > 0 {
		if strings.TrimSpace(status) == sysin.CollectEventStatusFailed && strings.TrimSpace(errorMessage) != "" {
			return "failed", errorMessage
		}
		return "caching", fmt.Sprintf("%d 个媒体待缓存", summary.Pending)
	}
	if summary.Ready > 0 {
		return "cached", fmt.Sprintf("%d 个媒体已缓存", summary.Ready)
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

func (s *sSysPublish) downloadTelegramMedia(ctx context.Context, tenantId int64, accountId int64, tgAccountId int64, item collectMediaItem) (*collectDownloadedMedia, error) {
	if tgAccountId <= 0 {
		return nil, gerror.New("账号采集媒体缺少TG账号")
	}
	task, err := collectorservice.AccountTasks().SubmitAndWait(ctx, &collectorin.AccountTaskSubmit{
		TenantID: tenantId, AccountID: tgAccountId, TaskType: collectorin.AccountTaskTypeMediaDownload,
		TaskKey: accountMediaDownloadTaskKey(tgAccountId, item), Priority: collectorin.EventPriorityRealtime,
		MediaOwnerAccountID: accountId, Media: ptrCollectorMediaItem(collectorMediaItemFromCollect(item)), MaxAttempts: 5,
	}, time.Second)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, newCollectMediaFairnessRetryError("账号媒体下载任务等待执行", 3*time.Second)
		}
		return nil, gerror.Wrap(err, "提交账号媒体下载任务失败")
	}
	switch task.Status {
	case collectorin.AccountTaskStatusCompleted:
		result := task.MediaResult
		if result.ErrorCode == "source_gone" {
			return nil, gerror.Wrap(errCollectMediaSourceGone, firstNonEmpty(result.ErrorMessage, "TG原消息已删除或已无可用媒体"))
		}
		if strings.TrimSpace(result.StoragePath) == "" && strings.TrimSpace(result.FileURL) == "" {
			return nil, gerror.New("账号媒体下载任务未返回共享存储地址")
		}
		return &collectDownloadedMedia{
			AttachmentId: result.AttachmentID,
			FileUrl:      strings.TrimSpace(result.FileURL),
			Path:         firstNonEmpty(result.StoragePath, result.FileURL),
			MetaJson:     result.Media.DebugMetaJSON,
			Item:         collectMediaItemFromCollector(result.Media),
		}, nil
	case collectorin.AccountTaskStatusDead, collectorin.AccountTaskStatusCancelled:
		return nil, gerror.New(firstNonEmpty(task.ErrorMessage, "账号媒体下载任务已终止"))
	default:
		return nil, newCollectMediaFairnessRetryError("账号媒体下载任务等待执行", 3*time.Second)
	}
}

func ptrCollectorMediaItem(item collectorin.CollectorMediaItem) *collectorin.CollectorMediaItem {
	return &item
}

func accountMediaDownloadTaskKey(tgAccountID int64, item collectMediaItem) string {
	identity := fmt.Sprintf("%d|%s|%s|%d|%d|%d", tgAccountID, strings.TrimSpace(item.Type), strings.TrimSpace(item.FileId), item.SourceMediaId, item.SourceDCId, item.SourceSize)
	return fmt.Sprintf("media:%x", sha256.Sum256([]byte(identity)))
}

func (s *sSysPublish) downloadBotTelegramMedia(ctx context.Context, tenantId int64, botId int64, item collectMediaItem) (*collectDownloadedMedia, error) {
	if botId <= 0 {
		return nil, gerror.New("Bot采集媒体缺少Bot ID")
	}
	fileID := strings.TrimSpace(item.FileId)
	if fileID == "" {
		return nil, gerror.New("Bot采集媒体缺少File ID")
	}

	var botRow struct {
		BotToken string `json:"botToken"`
	}
	if err := g.DB().Model(publishBotTable).Safe().Ctx(ctx).
		Fields("bot_token").
		Where("id", botId).
		Where("tenant_id IN(0, ?)", tenantId).
		Where("status", 1).
		WhereNull("deleted_at").
		Scan(&botRow); err != nil {
		return nil, gerror.Wrap(err, "读取Bot媒体下载凭证失败")
	}
	if strings.TrimSpace(botRow.BotToken) == "" {
		return nil, gerror.New("Bot媒体下载凭证不存在")
	}
	return s.downloadBotTelegramMediaWithToken(ctx, botId, botRow.BotToken, item)
}

func (s *sSysPublish) downloadBotTelegramMediaWithToken(ctx context.Context, botId int64, botToken string, item collectMediaItem) (*collectDownloadedMedia, error) {
	botToken = strings.TrimSpace(botToken)
	if botId <= 0 || botToken == "" {
		return nil, gerror.New("Bot媒体下载凭证不存在")
	}
	fileID := strings.TrimSpace(item.FileId)
	if fileID == "" {
		return nil, gerror.New("Bot采集媒体缺少File ID")
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return nil, gerror.Wrap(err, "创建Bot媒体下载客户端失败")
	}
	file, err := bot.GetFile(ctx, &tgbot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, gerror.Wrap(err, "读取Bot媒体文件信息失败")
	}
	if file == nil || strings.TrimSpace(file.FilePath) == "" {
		return nil, gerror.New("Bot媒体文件路径为空")
	}

	remoteSource := bot.FileDownloadLink(file)
	cacheSource := fmt.Sprintf("bot:%d:%s", botId, firstNonEmpty(file.FileUniqueID, fileID))
	cacheKey := mediaFileCacheKey(&telegramMediaItem{
		MediaType:   listenerTelegramMediaType(item.Type),
		StoragePath: fileID,
		TgFileId:    fileID,
		AssetHash:   cacheSource,
	}, cacheSource)
	conf, err := service.SysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, gerror.Wrap(err, "读取Bot媒体下载网络配置失败")
	}
	client, err := telegramHTTPClient(conf.ProxyUrl)
	if err != nil {
		return nil, gerror.Wrap(err, "创建Bot媒体下载网络客户端失败")
	}
	path, err := cachedRemoteMediaFileWithMetaSourceAndDownloader(ctx, cacheKey, remoteSource, cacheSource, collectMediaExt(item.Type, ""), func(downloadCtx context.Context, source string, filePath string) error {
		return downloadMediaFileCacheWithClient(downloadCtx, client, source, filePath)
	})
	if err != nil {
		return nil, gerror.Wrap(err, "下载Bot媒体文件失败")
	}
	if strings.TrimSpace(path) == "" {
		return nil, gerror.New("Bot媒体文件下载完成但缓存路径为空")
	}
	md5Value, mediaSize, mimeType, err := calculateTelegramMediaFingerprint(path, item.Type, item.SourceMimeType)
	if err != nil {
		return nil, err
	}
	if cached, ok, cacheErr := lookupTelegramCollectorMediaCache(ctx, item.Type, md5Value, mediaSize, mimeType); cacheErr != nil {
		return nil, cacheErr
	} else if ok {
		item.FileUrl = telegramCollectorMediaCacheURL(cached)
		item.StoragePath = cached.StoragePath
		item.FileMd5 = md5Value
		item.SourceSize = mediaSize
		item.SourceMimeType = mimeType
		item.FilePhash = cached.PHash
		item.PosterUrl = firstNonEmpty(cached.PosterURL, cached.PosterStoragePath)
		return &collectDownloadedMedia{FileUrl: item.FileUrl, Path: item.StoragePath, Item: item}, nil
	}
	attachment, err := s.uploadCollectMediaToStorage(ctx, item.Type, path)
	if err != nil {
		return nil, gerror.Wrap(err, "Bot采集媒体上传云存储失败")
	}
	if attachment == nil || (strings.TrimSpace(attachment.FileUrl) == "" && strings.TrimSpace(attachment.Path) == "") {
		return nil, gerror.New("Bot采集媒体上传云存储未返回有效地址")
	}
	item.FileUrl = strings.TrimSpace(attachment.FileUrl)
	item.StoragePath = normalizeStoredMediaPath(attachment.Path)
	item.FileMd5 = md5Value
	item.SourceSize = mediaSize
	item.SourceMimeType = mimeType
	return &collectDownloadedMedia{
		AttachmentId: attachment.Id,
		FileUrl:      item.FileUrl,
		Path:         firstNonEmpty(item.StoragePath, item.FileUrl),
		Item:         item,
	}, nil
}

func collectMediaSourceGone(err error) bool {
	return err != nil && errors.Is(err, errCollectMediaSourceGone)
}

func (s *sSysPublish) discardCollectEventGroup(ctx context.Context, eventId int64, reason string) error {
	if eventId <= 0 {
		return nil
	}
	profileIds, err := s.collectEventProfileIds(ctx, eventId)
	if err != nil {
		return err
	}
	for _, profileId := range profileIds {
		if err = s.deleteMediaPHashBucketByProfileId(ctx, profileId); err != nil {
			return gerror.Wrapf(err, "清理已删除TG来源资料PHash失败 profileId:%d", profileId)
		}
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, profileId := range profileIds {
			for _, table := range []string{
				"hg_content_media",
				"hg_youban_publish_media",
				"hg_youban_publish_note_index",
				"hg_youban_publish_channel_profile",
				"hg_content_source_map",
				"hg_youban_publish_profile_state",
			} {
				if _, deleteErr := tx.Model(table).Safe().Ctx(ctx).Where("profile_id", profileId).Delete(); deleteErr != nil {
					return gerror.Wrapf(deleteErr, "清理已删除TG来源资料关联失败 table:%s profileId:%d", table, profileId)
				}
			}
			if _, deleteErr := tx.Model("hg_content_profile").Safe().Ctx(ctx).Where("id", profileId).Delete(); deleteErr != nil {
				return gerror.Wrapf(deleteErr, "删除已删除TG来源资料失败 profileId:%d", profileId)
			}
		}
		for _, table := range []string{
			pdao.YoubanPublishCollectEventMedia.Table(),
			pdao.YoubanPublishCollectEventLog.Table(),
			pdao.YoubanPublishCollectReview.Table(),
			pdao.YoubanPublishCollectDispatch.Table(),
			"hg_youban_publish_collect_media_stat",
		} {
			if _, err := tx.Model(table).Where("event_id", eventId).Delete(); err != nil {
				return gerror.Wrapf(err, "丢弃TG来源资料组失败 table:%s", table)
			}
		}
		if _, err := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Where("id", eventId).Delete(); err != nil {
			return gerror.Wrap(err, "丢弃TG来源资料组事件失败")
		}
		g.Log().Warningf(ctx, "TG来源消息已删除，资料组已丢弃 eventId:%d reason:%s", eventId, reason)
		return nil
	})
}

func (s *sSysPublish) collectEventProfileIds(ctx context.Context, eventId int64) ([]int64, error) {
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("tenant_id", "account_id").
		Where("id", eventId).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG来源资料归属失败")
	}
	if event.IsEmpty() {
		return nil, nil
	}
	dispatches, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("profile_id").
		Where("event_id", eventId).
		Where("profile_id > 0").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG来源资料失败")
	}
	profileIds := make([]int64, 0, len(dispatches))
	for _, dispatch := range dispatches {
		profileId := dispatch["profile_id"].Int64()
		if profileId <= 0 {
			continue
		}
		state, stateErr := g.DB().Model("hg_youban_publish_profile_state").Safe().Ctx(ctx).
			Where("profile_id", profileId).
			Where("tenant_id", event["tenant_id"].Int64()).
			Where("account_id", event["account_id"].Int64()).
			WhereNull("deleted_at").
			One()
		if stateErr != nil {
			return nil, gerror.Wrapf(stateErr, "校验TG来源资料归属失败 profileId:%d", profileId)
		}
		if !state.IsEmpty() {
			profileIds = append(profileIds, profileId)
		}
	}
	return uniqueIds(profileIds), nil
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
	// Basic groups use PeerChat and have no channel access hash. Their realtime
	// updates may carry the positive chat ID, while the configured source uses
	// the conventional negative form (for example -5596823874).
	if peer, ok := s.collectBasicGroupInputPeer(ctx, tenantId, tgAccountId, chatId); ok {
		return peer, nil
	}
	id, parseErr := strconv.ParseInt(strings.TrimSpace(chatId), 10, 64)
	if parseErr == nil && id > 0 {
		return &tg.InputPeerUser{UserID: id}, nil
	}
	return nil, gerror.New("账号采集媒体无法解析原消息会话")
}

func (s *sSysPublish) collectBasicGroupInputPeer(ctx context.Context, tenantId int64, tgAccountId int64, chatId string) (tg.InputPeerClass, bool) {
	ids := tgChannelCacheLookupIds(chatId)
	if raw := strings.TrimSpace(chatId); raw != "" && !strings.HasPrefix(raw, "-") {
		ids = append(ids, "-"+raw)
	}
	if len(ids) == 0 {
		return nil, false
	}
	var source struct {
		SourceChatId string `orm:"source_chat_id"`
	}
	if err := g.DB().Model(publishCollectSourceTable).Safe().Ctx(ctx).
		Fields("source_chat_id").
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		WhereIn("source_chat_id", uniqueStrings(ids)).
		WhereNull("deleted_at").
		Scan(&source); err != nil {
		return nil, false
	}
	id, ok := parseBasicGroupChatID(source.SourceChatId)
	if !ok {
		return nil, false
	}
	return &tg.InputPeerChat{ChatID: id}, true
}

func parseBasicGroupChatID(value string) (int64, bool) {
	raw := strings.TrimSpace(value)
	if !strings.HasPrefix(raw, "-") || strings.HasPrefix(raw, "-100") {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(raw, "-"), 10, 64)
	return id, err == nil && id > 0
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
	Item         collectMediaItem
}

type collectMediaCacheResult struct {
	path     string
	metaJSON string
}

func (s *sSysPublish) acquireCollectMediaDownloadSlots(ctx context.Context, tenantId int64, tgAccountId int64) (func(), error) {
	if tenantId <= 0 {
		return func() {}, gerror.New("采集媒体下载缺少租户")
	}
	globalLimit := g.Cfg().MustGet(ctx, "youbanPublish.collect.globalMediaConcurrency", 8).Int()
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
	if s.collectMediaSlots == nil || cap(s.collectMediaSlots) != globalLimit {
		s.collectMediaSlots = make(chan struct{}, globalLimit)
	}
	globalSlots := s.collectMediaSlots
	var accountSlots chan struct{}
	if tgAccountId > 0 {
		if s.collectMediaAccounts == nil {
			s.collectMediaAccounts = make(map[string]chan struct{})
		}
		accountKey := collectMediaAccountKey(tenantId, tgAccountId)
		accountSlots = s.collectMediaAccounts[accountKey]
		if accountSlots == nil || cap(accountSlots) != accountLimit {
			accountSlots = make(chan struct{}, accountLimit)
			s.collectMediaAccounts[accountKey] = accountSlots
		}
	}
	s.collectMediaMu.Unlock()
	if accountSlots != nil {
		select {
		case accountSlots <- struct{}{}:
		case <-ctx.Done():
			return func() {}, ctx.Err()
		}
	}
	select {
	case globalSlots <- struct{}{}:
	case <-ctx.Done():
		if accountSlots != nil {
			<-accountSlots
		}
		return func() {}, ctx.Err()
	}
	var lease *collectMediaAccountLease
	if tgAccountId > 0 {
		var acquired bool
		var err error
		lease, acquired, err = acquireCollectMediaAccountLease(ctx, tenantId, tgAccountId, accountLimit)
		if err != nil {
			<-globalSlots
			<-accountSlots
			return func() {}, gerror.Wrap(err, "获取TG账号媒体分布式并发租约失败")
		}
		if !acquired {
			<-globalSlots
			<-accountSlots
			return func() {}, newCollectMediaFairnessRetryError("TG账号媒体并发已满，等待公平调度", 3*time.Second)
		}
	}
	return func() {
		if lease != nil {
			lease.Release()
		}
		<-globalSlots
		if accountSlots != nil {
			<-accountSlots
		}
	}, nil
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
