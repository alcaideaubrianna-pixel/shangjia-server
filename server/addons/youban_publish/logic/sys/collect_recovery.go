package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/tasklog"
)

const (
	collectHistoryRecoverAfter = 2 * time.Minute
	collectEventRecoverAfter   = 10 * time.Second
	collectMediaRecoverAfter   = 35 * time.Minute
)

func (s *sSysPublish) runCollectRecovery(ctx context.Context) {
	time.Sleep(12 * time.Second)
	s.recoverCollectOnce(ctx)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverCollectOnce(ctx)
		}
	}
}

func (s *sSysPublish) recoverCollectOnce(ctx context.Context) {
	mediaBatchSize := collectMediaRecoveryBatchSize(ctx)
	if err := s.cleanupCollectEventsOlderThan(ctx, tasklog.RetentionDays(), 1000); err != nil {
		g.Log().Warningf(ctx, "清理过期采集事件失败：%+v", err)
	}
	if err := s.recoverStaleCollectMediaRows(ctx, mediaBatchSize); err != nil {
		g.Log().Warningf(ctx, "恢复超时采集媒体失败：%+v", err)
	}
	if err := s.recoverInvalidReadyCollectMedia(ctx, mediaBatchSize); err != nil {
		g.Log().Warningf(ctx, "恢复无持久化文件的采集媒体失败：%+v", err)
	}
	if err := s.recoverPendingCollectMedia(ctx, mediaBatchSize); err != nil {
		g.Log().Warningf(ctx, "恢复待处理采集媒体任务失败：%+v", err)
	}
	if err := s.recoverCollectHistoryTasks(ctx, 20); err != nil {
		g.Log().Warningf(ctx, "恢复历史采集任务失败：%+v", err)
	}
	if err := s.ignoreStaleCollectEventsForDisabledSources(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "清理已关闭采集源事件失败：%+v", err)
	}
	if err := s.recoverCollectEvents(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复采集事件处理失败：%+v", err)
	}
	if err := s.recoverProcessedCollectEvents(ctx, 200); err != nil {
		g.Log().Warningf(ctx, "恢复采集事件完成状态失败：%+v", err)
	}
	if err := s.recoverCollectPublishTasks(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复采集推送任务失败：%+v", err)
	}
}

// recoverInvalidReadyCollectMedia repairs legacy rows that treated a Telegram
// file_id as a durable cache. They must re-enter the normal download queue;
// otherwise the parent event can remain in media_pending indefinitely.
func (s *sSysPublish) recoverInvalidReadyCollectMedia(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 1000
	}
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	rows, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).As("m").
		Fields("m."+mediaCols.Id).
		Where("m."+mediaCols.CacheStatus, collectMediaCacheReady).
		Where("COALESCE(TRIM(m." + mediaCols.StoragePath + "), '') = ''").
		Where("COALESCE(TRIM(m." + mediaCols.FileUrl + "), '') = ''").
		Where("NOT (COALESCE(TRIM(m." + mediaCols.BackupChatId + "), '') <> '' AND m." + mediaCols.BackupMessageId + " > 0)").
		Where(enabledCollectSourceExistsSQL("m")).
		OrderAsc("m." + mediaCols.Id).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取无持久化文件的采集媒体失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row[mediaCols.Id].Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	now := gtime.Now()
	result, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		WhereIn(mediaCols.Id, ids).
		Where(mediaCols.CacheStatus, collectMediaCacheReady).
		Where("COALESCE(TRIM(" + mediaCols.StoragePath + "), '') = ''").
		Where("COALESCE(TRIM(" + mediaCols.FileUrl + "), '') = ''").
		Where("NOT (COALESCE(TRIM(" + mediaCols.BackupChatId + "), '') <> '' AND " + mediaCols.BackupMessageId + " > 0)").
		Data(g.Map{
			mediaCols.CacheStatus:   collectMediaCachePending,
			collectMediaNextRetryAt: nil,
			mediaCols.ErrorMessage:  "历史媒体缺少持久化文件，已自动恢复下载",
			mediaCols.UpdatedAt:     now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "重置无持久化文件的采集媒体失败")
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		g.Log().Warningf(ctx, "已恢复无持久化文件的采集媒体 count:%d", affected)
	}
	return nil
}

// Submission/duplicate notices are terminal and must never enter media or
// publish processing. Clean up old pending rows as well as new ones so a
// backlog cannot consume the collection queues.
func (s *sSysPublish) ignoreCollectNoticeEvents(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 500
	}
	e := pdao.YoubanPublishCollectEvent.Columns()
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Fields(e.Id, e.RawText).
		WhereIn(e.Status, []string{sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusWaitingOrder, sysin.CollectEventStatusPrechecked, sysin.CollectEventStatusMediaPending}).
		WhereNull(e.ProcessedAt).OrderAsc(e.UpdatedAt).Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取采集通知事件失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if profileMessageIgnoredNotice(row[e.RawText].String()) {
			ids = append(ids, row[e.Id].Int64())
		}
	}
	if len(ids) == 0 {
		return nil
	}
	now := gtime.Now()
	_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).WhereIn(e.Id, ids).Data(g.Map{
		e.Status: sysin.CollectEventStatusIgnored, e.ErrorMessage: "删除关键字忽略:投稿/重复通知", e.ProcessedAt: now, e.UpdatedAt: now,
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "标记采集通知事件失败")
	}
	_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).WhereIn("event_id", ids).Data(g.Map{"cache_status": collectMediaCacheCanceled, "error_message": "删除关键字忽略:投稿/重复通知", "updated_at": now}).Update()
	g.Log().Infof(ctx, "已忽略采集通知事件 count:%d", len(ids))
	return nil
}

// recoverBotCollectMedia is deliberately event-scoped. Older source-scoped
// recovery can keep a busy BOT source alive without ever putting a particular
// media group back on the media queue. Re-enqueueing each stale BOT event
// lets the normal BOT downloader resume the group independently.
func (s *sSysPublish) recoverBotCollectMedia(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 1000
	}
	e := pdao.YoubanPublishCollectEvent.Columns()
	deadline := gtime.Now().Add(-collectEventRecoverAfter)
	model := pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
		Fields("e.*").
		Where("e."+e.SourceType, sysin.CollectSourceTypeBot).
		WhereIn("e."+e.Status, []string{sysin.CollectEventStatusMediaPending, sysin.CollectEventStatusMediaReady, sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusPending, sysin.CollectEventStatusFailed}).
		WhereNull("e."+e.ProcessedAt).
		WhereLTE("e."+e.UpdatedAt, deadline).
		Where(enabledCollectSourceExistsSQL("e"))
	// Keep ordering and pagination on the outer event query. GoFrame may attach
	// later Order/Limit calls to an EXISTS expression when its predicate is
	// built inline, producing invalid PostgreSQL SQL (ORDER inside EXISTS).
	model = model.Where("e."+e.Id+" IN (SELECT m.event_id FROM "+pdao.YoubanPublishCollectEventMedia.Table()+" m WHERE m.cache_status IN (?,?))", collectMediaCachePending, collectMediaCacheDownloading)
	rows, err := model.OrderAsc("e." + e.UpdatedAt).Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复BOT媒体事件失败")
	}
	requeued := 0
	for _, row := range rows {
		payload := collectMediaQueuePayloadFromEvent(row)
		if payload.EventId <= 0 || payload.TenantId <= 0 || payload.SourceId <= 0 {
			continue
		}
		queued, enqueueErr := s.enqueueCollectMediaCacheDeferred(ctx, payload, 0)
		if enqueueErr != nil {
			g.Log().Warningf(ctx, "BOT媒体事件重新入队失败 eventId:%d err:%+v", payload.EventId, enqueueErr)
			continue
		}
		if queued {
			requeued++
			g.Log().Infof(ctx, "BOT媒体事件已按事件重新入队 eventId:%d sourceId:%d", payload.EventId, payload.SourceId)
		}
	}
	if requeued > 0 {
		g.Log().Infof(ctx, "BOT媒体事件恢复完成 count:%d", requeued)
	}
	return nil
}

// RepairBotCollectMedia exposes the one-shot historical repair for the CLI.
func RepairBotCollectMedia(ctx context.Context, limit int) error {
	return NewSysPublish().recoverBotCollectMedia(ctx, limit)
}

// IgnoreCollectNoticeEvents exposes the one-shot notification cleanup for the CLI.
func IgnoreCollectNoticeEvents(ctx context.Context, limit int) error {
	return NewSysPublish().ignoreCollectNoticeEvents(ctx, limit)
}

func (s *sSysPublish) ignoreStaleCollectEventsForDisabledSources(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	deadline := gtime.Now().Add(-collectEventRecoverAfter)
	statuses := []string{
		sysin.CollectEventStatusPending,
		sysin.CollectEventStatusGroupCollect,
		sysin.CollectEventStatusWaitingOrder,
		sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusFailed,
	}
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
		Fields("e.id").
		WhereIn("e.status", statuses).
		WhereLTE("e.updated_at", deadline).
		Where("NOT " + enabledCollectSourceExistsSQL("e")).
		OrderAsc("e.updated_at").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取已关闭采集源事件失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row["id"].Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	now := gtime.Now()
	if _, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).WhereIn("id", ids).Data(g.Map{
		"status":        sysin.CollectEventStatusIgnored,
		"error_message": "采集源已关闭，事件不再处理",
		"processed_at":  now,
		"updated_at":    now,
	}).Update(); err != nil {
		return gerror.Wrap(err, "标记已关闭采集源事件失败")
	}
	return nil
}

func (s *sSysPublish) recoverStaleCollectMediaRows(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 1000
	}
	deadline := gtime.Now().Add(-collectMediaRecoverAfter)
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	rows, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).As("m").
		Fields("m."+mediaCols.Id, "m."+mediaCols.EventId).
		Where("m."+mediaCols.CacheStatus, collectMediaCacheDownloading).
		WhereLTE("m."+mediaCols.UpdatedAt, deadline).
		Where(enabledCollectSourceExistsSQL("m")).
		OrderAsc("m." + mediaCols.UpdatedAt).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取超时采集媒体失败")
	}
	if len(rows) == 0 {
		return nil
	}

	mediaIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.IsEmpty() || row[mediaCols.Id].Int64() <= 0 {
			continue
		}
		mediaIds = append(mediaIds, row[mediaCols.Id].Int64())
	}
	if len(mediaIds) == 0 {
		return nil
	}
	_, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		WhereIn(mediaCols.Id, mediaIds).
		Where(mediaCols.CacheStatus, collectMediaCacheDownloading).
		WhereLTE(mediaCols.UpdatedAt, deadline).
		Data(g.Map{
			mediaCols.CacheStatus:   collectMediaCachePending,
			collectMediaNextRetryAt: nil,
			mediaCols.ErrorMessage:  "媒体缓存任务超时，已自动恢复重试",
			mediaCols.UpdatedAt:     gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "重置超时采集媒体失败")
	}
	g.Log().Warningf(ctx, "已恢复超时采集媒体 count:%d", len(mediaIds))
	return nil
}

// RepairStaleCollectMedia performs the same bounded recovery as the runtime
// watchdog, but runs synchronously for an operator after a deployment or
// queue outage. It first normalizes timed-out downloads and then immediately
// requeues the affected event-scoped media tasks.
func RepairStaleCollectMedia(ctx context.Context, limit int) error {
	service := NewSysPublish()
	if limit <= 0 {
		limit = 1000
	}
	if err := service.recoverStaleCollectMediaRows(ctx, limit); err != nil {
		return err
	}
	return service.recoverPendingCollectMedia(ctx, limit)
}

func (s *sSysPublish) recoverPendingCollectMedia(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 1000
	}
	now := gtime.Now()
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	mediaTable := pdao.YoubanPublishCollectEventMedia.Table()
	candidateLimit := limit * 4
	if candidateLimit > 2000 {
		candidateLimit = 2000
	}
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
		Where("e."+eventCols.Status, sysin.CollectEventStatusMediaPending).
		WhereNull("e."+eventCols.ProcessedAt).
		Where(enabledCollectSourceExistsSQL("e")).
		Where("EXISTS (SELECT 1 FROM "+mediaTable+" m WHERE m.event_id=e."+eventCols.Id+" AND m.cache_status=? AND (m.next_retry_at IS NULL OR m.next_retry_at<=?))", collectMediaCachePending, now).
		OrderAsc("e." + eventCols.UpdatedAt).
		Limit(candidateLimit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复采集媒体事件失败")
	}
	if len(rows) == 0 {
		return nil
	}

	recovered := 0
	for _, row := range fairCollectMediaRecoveryRows(rows, limit) {
		if row.IsEmpty() {
			continue
		}
		payload := collectMediaQueuePayloadFromEvent(row)
		if payload.EventId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 || payload.SourceId <= 0 {
			continue
		}
		// Recovery must bypass Asynq's 24-hour unique key. A lost or stuck
		// unique task otherwise prevents this media event from being requeued.
		enqueued, enqueueErr := s.enqueueCollectMediaCacheDeferred(ctx, payload, 0)
		if enqueueErr != nil {
			g.Log().Warningf(ctx, "恢复待处理采集媒体任务投递失败 eventId:%d sourceId:%d err:%+v", payload.EventId, payload.SourceId, enqueueErr)
			continue
		}
		if !enqueued {
			observeCollectMediaScheduler(ctx, "recovery_enqueue_skipped", payload.TenantId, payload.TgAccountId)
			continue
		}
		observeCollectMediaScheduler(ctx, "recovery_reenqueued", payload.TenantId, payload.TgAccountId)
		if _, updateErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Where(eventCols.Id, payload.EventId).
			Where(eventCols.Status, sysin.CollectEventStatusMediaPending).
			Data(g.Map{eventCols.UpdatedAt: now}).Update(); updateErr != nil {
			g.Log().Warningf(ctx, "更新待处理采集媒体恢复心跳失败 eventId:%d err:%+v", payload.EventId, updateErr)
			continue
		}
		recovered++
	}
	if recovered > 0 {
		g.Log().Infof(ctx, "已重新投递待处理采集媒体任务 count:%d", recovered)
	}
	return nil
}

func fairCollectMediaRecoveryRows(rows gdb.Result, limit int) gdb.Result {
	if limit <= 0 || len(rows) <= 1 {
		return rows
	}
	accountOrder := make([]string, 0)
	byAccount := make(map[string]gdb.Result)
	for _, row := range rows {
		if row.IsEmpty() {
			continue
		}
		key := collectMediaAccountKey(row["tenant_id"].Int64(), row["tg_account_id"].Int64())
		if _, exists := byAccount[key]; !exists {
			accountOrder = append(accountOrder, key)
		}
		byAccount[key] = append(byAccount[key], row)
	}
	result := make(gdb.Result, 0, minInt(limit, len(rows)))
	for len(result) < limit {
		progressed := false
		for _, key := range accountOrder {
			accountRows := byAccount[key]
			if len(accountRows) == 0 {
				continue
			}
			result = append(result, accountRows[0])
			byAccount[key] = accountRows[1:]
			progressed = true
			if len(result) >= limit {
				break
			}
		}
		if !progressed {
			break
		}
	}
	return result
}

func collectMediaRecoveryBatchSize(ctx context.Context) int {
	batchSize := g.Cfg().MustGet(ctx, "youbanPublish.collect.mediaRecoveryBatchSize", 200).Int()
	if batchSize < 20 {
		return 20
	}
	if batchSize > 500 {
		return 500
	}
	return batchSize
}

func (s *sSysPublish) recoverCollectHistoryTasks(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	now := gtime.Now()
	stale := now.Add(-collectHistoryRecoverAfter)
	rows, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).As("h").
		Where(enabledCollectSourceExistsSQL("h")).
		Where(
			"(status=? OR (status=? AND updated_at<=?) OR (status=? AND (next_run_at IS NULL OR next_run_at<=?)) OR (status=? AND (error_message LIKE ? OR error_message LIKE ? OR error_message LIKE ? OR error_message LIKE ?)))",
			sysin.CollectHistoryTaskStatusPending,
			sysin.CollectHistoryTaskStatusRunning,
			stale,
			sysin.CollectHistoryTaskStatusPaused,
			now,
			sysin.CollectHistoryTaskStatusFailed,
			"%TG账号连接正在使用，拒绝创建第二个客户端%",
			"%client closed%context canceled%",
			"%DC is closed%",
			"%engine forcibly closed%",
		).
		OrderAsc("h.updated_at").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复历史采集任务失败")
	}
	for _, row := range rows {
		if row.IsEmpty() || row["id"].Int64() <= 0 {
			continue
		}
		if err = s.recoverCollectHistoryTask(ctx, row); err != nil {
			g.Log().Warningf(ctx, "恢复历史采集任务失败 task:%d err:%+v", row["id"].Int64(), err)
		}
	}
	return nil
}

func (s *sSysPublish) recoverCollectHistoryTask(ctx context.Context, row gdb.Record) error {
	taskId := row["id"].Int64()
	if taskId <= 0 {
		return nil
	}
	if strings.Contains(row["error_message"].String(), "TG账号连接正在使用，拒绝创建第二个客户端") {
		g.Log().Errorf(ctx, "历史采集任务曾因重复创建TG客户端失败，正在恢复 task:%d accountId:%d tgAccountId:%d err:%s", taskId, row["account_id"].Int64(), row["tg_account_id"].Int64(), row["error_message"].String())
	}
	_, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).Where("id", taskId).Data(g.Map{
		"status":        sysin.CollectHistoryTaskStatusPending,
		"error_message": "",
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "重置历史采集任务失败")
	}
	s.appendCollectHistoryLog(ctx, taskId, row["tenant_id"].Int64(), row["account_id"].Int64(), "info", "requeue", "历史采集任务恢复并重新投递", nil)
	return s.enqueueCollectHistoryTask(ctx, taskId, 0)
}

func (s *sSysPublish) recoverCollectEvents(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	deadline := gtime.Now().Add(-collectEventRecoverAfter)
	statuses := []string{sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusWaitingOrder, sysin.CollectEventStatusPrechecked, sysin.CollectEventStatusMediaPending, sysin.CollectEventStatusMediaReady, sysin.CollectEventStatusFailed}
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
		Fields("e.*").
		WhereIn("e.status", statuses).
		WhereLTE("e.updated_at", deadline).
		Where(enabledCollectSourceExistsSQL("e")).
		OrderAsc("e.updated_at").
		OrderAsc("e.id").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复采集事件失败")
	}
	for _, row := range rows {
		if row.IsEmpty() || row["id"].Int64() <= 0 || !shouldRecoverCollectEvent(row) {
			continue
		}
		if row["status"].String() == sysin.CollectEventStatusMediaPending {
			canResume, mediaErr := s.collectEventMediaCanResume(ctx, row["id"].Int64())
			if mediaErr != nil {
				g.Log().Warningf(ctx, "检查采集事件媒体恢复条件失败 event:%d err:%+v", row["id"].Int64(), mediaErr)
				continue
			}
			if !canResume {
				continue
			}
		}
		claimData := g.Map{"updated_at": gtime.Now()}
		if row["status"].String() == sysin.CollectEventStatusFailed {
			claimData["status"] = sysin.CollectEventStatusPending
			claimData["error_message"] = ""
			claimData["processed_at"] = nil
		}
		claimResult, claimErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Where("id", row["id"].Int64()).
			WhereLTE("updated_at", deadline).
			Data(claimData).Update()
		if claimErr != nil {
			g.Log().Warningf(ctx, "抢占待恢复采集事件失败 event:%d err:%+v", row["id"].Int64(), claimErr)
			continue
		}
		claimed, claimErr := claimResult.RowsAffected()
		if claimErr != nil {
			g.Log().Warningf(ctx, "确认待恢复采集事件抢占结果失败 event:%d err:%+v", row["id"].Int64(), claimErr)
			continue
		}
		if claimed == 0 {
			continue
		}
		queued, processErr := s.enqueueCollectProcessDeferred(ctx, collectProcessQueuePayload{
			EventId:      row["id"].Int64(),
			SourceId:     row["source_id"].Int64(),
			SourceChatId: row["source_chat_id"].String(),
			TenantId:     row["tenant_id"].Int64(),
			AccountId:    row["account_id"].Int64(),
		}, 0)
		if processErr != nil {
			g.Log().Warningf(ctx, "恢复采集事件投递失败 source:%d event:%d err:%+v", row["source_id"].Int64(), row["id"].Int64(), processErr)
			continue
		}
		if !queued {
			g.Log().Warningf(ctx, "恢复采集事件任务未实际入队 source:%d event:%d", row["source_id"].Int64(), row["id"].Int64())
			continue
		}
	}
	return nil
}

// RecoverCollectEvents runs one bounded recovery pass through the normal
// collection queue. It is intended for operational repair tooling.
func (s *sSysPublish) RecoverCollectEvents(ctx context.Context, limit int) error {
	return s.recoverCollectEvents(ctx, limit)
}

func enabledCollectSourceExistsSQL(ownerAlias string) string {
	return "EXISTS (SELECT 1 FROM " + pdao.YoubanPublishCollectSource.Table() + " s WHERE s.id=" + ownerAlias + ".source_id AND s.tenant_id=" + ownerAlias + ".tenant_id AND s.account_id=" + ownerAlias + ".account_id AND s.collect_enabled=1 AND s.status=1 AND s.deleted_at IS NULL)"
}

func (s *sSysPublish) collectEventMediaCanResume(ctx context.Context, eventId int64) (bool, error) {
	if eventId <= 0 {
		return false, nil
	}
	cols := pdao.YoubanPublishCollectEventMedia.Columns()
	rows, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields(cols.CacheStatus, cols.NextRetryAt, cols.StoragePath, cols.FileUrl, cols.BackupChatId, cols.BackupMessageId).
		Where(cols.EventId, eventId).
		All()
	if err != nil {
		return false, gerror.Wrapf(err, "读取采集事件媒体失败 event:%d", eventId)
	}
	return collectEventMediaRecordsCanResume(rows, gtime.Now()), nil
}

func collectEventMediaRecordsCanResume(rows gdb.Result, now *gtime.Time) bool {
	if len(rows) == 0 || now == nil {
		return false
	}
	cols := pdao.YoubanPublishCollectEventMedia.Columns()
	for _, row := range rows {
		status := strings.TrimSpace(row[cols.CacheStatus].String())
		if status == collectMediaCacheReady &&
			(strings.TrimSpace(row[cols.StoragePath].String()) != "" || strings.TrimSpace(row[cols.FileUrl].String()) != "" ||
				(strings.TrimSpace(row[cols.BackupChatId].String()) != "" && row[cols.BackupMessageId].Int64() > 0)) {
			continue
		}
		if status == collectMediaCacheReady {
			continue
		}
		if status == collectMediaCachePending {
			nextRetryAt := row[cols.NextRetryAt].GTime()
			if nextRetryAt == nil || !nextRetryAt.After(now) {
				continue
			}
		}
		return false
	}
	// A crash can happen after the last media row becomes ready but before the
	// parent event is re-enqueued. That state is immediately safe to resume.
	return true
}

func shouldRecoverCollectEvent(row gdb.Record) bool {
	status := strings.TrimSpace(row["status"].String())
	if status == sysin.CollectEventStatusPending || status == sysin.CollectEventStatusGroupCollect || status == sysin.CollectEventStatusWaitingOrder || status == sysin.CollectEventStatusPrechecked || status == sysin.CollectEventStatusMediaPending || status == sysin.CollectEventStatusMediaReady {
		return true
	}
	if status != sysin.CollectEventStatusFailed {
		return false
	}
	message := strings.ToLower(row["error_message"].String())
	for _, terminal := range []string{
		"未找到原消息",
		"无法刷新文件引用",
		"缺少原消息引用",
		"缺少下载元数据",
	} {
		if strings.Contains(message, strings.ToLower(terminal)) {
			return false
		}
	}
	return strings.Contains(message, "app_id") ||
		strings.Contains(message, "资料指纹索引正在初始化") ||
		strings.Contains(message, "账号采集媒体") ||
		strings.Contains(message, "媒体") ||
		strings.Contains(message, "上一条采集资料") ||
		strings.Contains(message, "验证视频暂未匹配到前序资料")
}

func (s *sSysPublish) recoverCollectPublishTasks(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	deadline := gtime.Now().Add(-collectEventRecoverAfter)
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("profile_id > 0").
		Where("status", sysin.CollectDispatchStatusPending).
		WhereLTE("updated_at", deadline).
		OrderAsc("updated_at").Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复采集资料分发失败")
	}
	repaired := 0
	for _, row := range rows {
		event, eventErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", row["event_id"].Int64()).One()
		if eventErr != nil || event.IsEmpty() {
			continue
		}
		rule, ruleErr := pdao.YoubanPublishCollectRule.Ctx(ctx).Where("id", row["rule_id"].Int64()).WhereNull("deleted_at").One()
		if ruleErr != nil || rule.IsEmpty() {
			continue
		}
		if err = s.submitCollectProfileDispatch(ctx, row["id"].Int64(), row["profile_id"].Int64(), event); err != nil {
			g.Log().Warningf(ctx, "恢复采集资料TG任务失败 dispatch:%d err:%+v", row["id"].Int64(), err)
			continue
		}
		repaired++
	}
	if repaired > 0 {
		g.Log().Infof(ctx, "已恢复采集资料推送：%d条", repaired)
	}
	return nil
}
