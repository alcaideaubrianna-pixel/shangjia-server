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
	if err := s.recoverCollectHistoryTasks(ctx, 20); err != nil {
		g.Log().Warningf(ctx, "恢复历史采集任务失败：%+v", err)
	}
	if err := s.recoverCollectEvents(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复采集事件处理失败：%+v", err)
	}
}

func (s *sSysPublish) recoverCollectHistoryTasks(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	now := gtime.Now()
	stale := now.Add(-collectHistoryRecoverAfter)
	rows, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).
		Where(
			"(status=? OR (status=? AND updated_at<=?) OR (status=? AND (next_run_at IS NULL OR next_run_at<=?)))",
			sysin.CollectHistoryTaskStatusPending,
			sysin.CollectHistoryTaskStatusRunning,
			stale,
			sysin.CollectHistoryTaskStatusPaused,
			now,
		).
		OrderAsc("updated_at").
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
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		WhereIn("status", []string{sysin.CollectEventStatusPending, sysin.CollectEventStatusFailed}).
		WhereLTE("updated_at", deadline).
		OrderAsc("updated_at").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复采集事件失败")
	}
	for _, row := range rows {
		if row.IsEmpty() || row["id"].Int64() <= 0 || !shouldRecoverCollectEvent(row) {
			continue
		}
		if err = s.processCollectEvent(ctx, row["id"].Int64(), row["tenant_id"].Int64(), row["account_id"].Int64()); err != nil {
			g.Log().Warningf(ctx, "恢复采集事件处理失败 event:%d err:%+v", row["id"].Int64(), err)
		}
	}
	return nil
}

func shouldRecoverCollectEvent(row gdb.Record) bool {
	status := strings.TrimSpace(row["status"].String())
	if status == sysin.CollectEventStatusPending {
		return true
	}
	if status != sysin.CollectEventStatusFailed {
		return false
	}
	message := strings.ToLower(row["error_message"].String())
	return strings.Contains(message, "app_id") ||
		strings.Contains(message, "账号采集媒体") ||
		strings.Contains(message, "媒体")
}
