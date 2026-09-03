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
	// Migration batches are bounded so normal recovery is not starved. Once
	// caught up, these become inexpensive indexed no-op queries.
	for batch := 0; batch < 5; batch++ {
		if err := s.backfillCollectDedupeLedger(ctx, 1000); err != nil {
			g.Log().Warningf(ctx, "回填采集永久去重账本失败 batch:%d err:%+v", batch+1, err)
			break
		}
	}
	if err := s.cleanupCollectEventsOlderThan(ctx, collectEventRetentionDays, 1000); err != nil {
		g.Log().Warningf(ctx, "清理过期采集事件失败：%+v", err)
	}
	if err := s.recoverStaleCollectMediaRows(ctx, mediaBatchSize); err != nil {
		g.Log().Warningf(ctx, "恢复超时采集媒体失败：%+v", err)
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
		enqueued, enqueueErr := s.enqueueCollectMediaCacheTask(ctx, payload, 0)
		if enqueueErr != nil {
			g.Log().Warningf(ctx, "恢复待处理采集媒体任务投递失败 eventId:%d sourceId:%d err:%+v", payload.EventId, payload.SourceId, enqueueErr)
			continue
		}
		if !enqueued {
			continue
		}
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
	sourceRows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
		Fields("e.source_id,MIN(e.updated_at) AS oldest_at").
		WhereIn("e.status", statuses).
		WhereLTE("e.updated_at", deadline).
		Where(enabledCollectSourceExistsSQL("e")).
		Group("e.source_id").
		OrderAsc("oldest_at").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复采集源失败")
	}
	remaining := limit
	for _, sourceRow := range sourceRows {
		if remaining <= 0 {
			break
		}
		sourceId := sourceRow["source_id"].Int64()
		sourceLimit := 10
		if sourceLimit > remaining {
			sourceLimit = remaining
		}
		rows, queryErr := s.collectRecoveryEventRows(ctx, sourceId, statuses, deadline, sourceLimit)
		if queryErr != nil {
			return queryErr
		}
		for _, row := range rows {
			if row.IsEmpty() || row["id"].Int64() <= 0 || !shouldRecoverCollectEvent(row) {
				continue
			}
			if row["status"].String() == sysin.CollectEventStatusMediaPending && !s.collectEventHasDueMedia(ctx, row["id"].Int64()) {
				continue
			}
			remaining--
			if processErr := s.enqueueCollectProcess(ctx, collectProcessQueuePayload{
				EventId:   row["id"].Int64(),
				SourceId:  row["source_id"].Int64(),
				TenantId:  row["tenant_id"].Int64(),
				AccountId: row["account_id"].Int64(),
			}, 0); processErr != nil {
				g.Log().Warningf(ctx, "恢复采集事件投递失败 event:%d err:%+v", row["id"].Int64(), processErr)
				continue
			}
			if _, updateErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
				Where("id", row["id"].Int64()).
				WhereLTE("updated_at", deadline).
				Data(g.Map{"updated_at": gtime.Now()}).Update(); updateErr != nil {
				g.Log().Warningf(ctx, "更新恢复采集事件心跳失败 event:%d err:%+v", row["id"].Int64(), updateErr)
			}
		}
	}
	return nil
}

func enabledCollectSourceExistsSQL(ownerAlias string) string {
	return "EXISTS (SELECT 1 FROM " + pdao.YoubanPublishCollectSource.Table() + " s WHERE s.id=" + ownerAlias + ".source_id AND s.tenant_id=" + ownerAlias + ".tenant_id AND s.account_id=" + ownerAlias + ".account_id AND s.collect_enabled=1 AND s.status=1 AND s.deleted_at IS NULL)"
}

func (s *sSysPublish) collectEventHasDueMedia(ctx context.Context, eventId int64) bool {
	if eventId <= 0 {
		return false
	}
	cols := pdao.YoubanPublishCollectEventMedia.Columns()
	row, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields(cols.Id).
		Where(cols.EventId, eventId).
		Where(cols.CacheStatus, collectMediaCachePending).
		Where("(next_retry_at IS NULL OR next_retry_at <= ?)", gtime.Now()).
		Limit(1).
		One()
	return err == nil && !row.IsEmpty()
}

func (s *sSysPublish) collectRecoveryEventRows(ctx context.Context, sourceId int64, statuses []string, deadline *gtime.Time, limit int) (gdb.Result, error) {
	mediaRows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("source_id", sourceId).
		Where("status", sysin.CollectEventStatusMediaPending).
		WhereLTE("updated_at", deadline).
		OrderAsc("updated_at").
		Limit(limit).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取待恢复采集媒体事件失败")
	}
	if len(mediaRows) >= limit {
		return mediaRows, nil
	}
	otherRows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("source_id", sourceId).
		WhereNot("status", sysin.CollectEventStatusMediaPending).
		WhereIn("status", statuses).
		WhereLTE("updated_at", deadline).
		OrderAsc("updated_at").
		Limit(limit - len(mediaRows)).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取待恢复采集事件失败")
	}
	return append(mediaRows, otherRows...), nil
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
