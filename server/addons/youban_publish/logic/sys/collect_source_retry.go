package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) retryCollectSourceAfterEnabled(ctx context.Context, sourceId int64, tenantId int64, accountId int64) {
	var source *sysin.CollectSourceModel
	err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		Scan(&source)
	if err != nil {
		g.Log().Warningf(ctx, "重新开启采集源后读取配置失败 sourceId:%d err:%+v", sourceId, err)
		return
	}
	if source == nil || source.Id <= 0 || source.CollectEnabled != 1 {
		return
	}
	if source.SourceType != sysin.CollectSourceTypeAccount || source.TgAccountId <= 0 {
		return
	}
	if err = s.resetAccountCollectCircuitForRetry(ctx, source.TgAccountId); err != nil {
		g.Log().Warningf(ctx, "重新开启采集源无法恢复TG账号连接 sourceId:%d tgAccountId:%d err:%+v", sourceId, source.TgAccountId, err)
		return
	}
	s.refreshAccountCollectSupervisor()
	if source.HistoryCollectEnabled != 1 {
		return
	}
	if err = s.retryCollectHistoryTaskForSource(ctx, source); err != nil {
		g.Log().Warningf(ctx, "重新开启采集源后恢复历史采集任务失败 sourceId:%d err:%+v", sourceId, err)
	}
}

func (s *sSysPublish) resetAccountCollectCircuitForRetry(ctx context.Context, tgAccountId int64) error {
	if tgAccountId <= 0 {
		return gerror.New("TG账号无效")
	}
	s.restoreAccountCollectCircuit(ctx, tgAccountId)
	s.accountCircuitMu.Lock()
	state, exists := s.accountCircuits[tgAccountId]
	if exists && state.permanent {
		s.accountCircuitMu.Unlock()
		return gerror.New("TG账号授权已失效，请重新登录后再开启采集")
	}
	delete(s.accountCircuits, tgAccountId)
	s.accountCircuitMu.Unlock()
	s.clearPersistedAccountCollectCircuit(ctx, tgAccountId)
	return nil
}

func (s *sSysPublish) retryCollectHistoryTaskForSource(ctx context.Context, source *sysin.CollectSourceModel) error {
	if source == nil || source.Id <= 0 {
		return nil
	}
	row, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).
		Where("source_id", source.Id).
		Where("tenant_id", source.TenantId).
		Where("account_id", source.AccountId).
		WhereIn("status", []string{
			sysin.CollectHistoryTaskStatusPending,
			sysin.CollectHistoryTaskStatusRunning,
			sysin.CollectHistoryTaskStatusPaused,
			sysin.CollectHistoryTaskStatusFailed,
		}).
		OrderDesc("id").
		Limit(1).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取采集源历史任务失败")
	}
	if row.IsEmpty() {
		_, err = s.createCollectHistoryTask(ctx, source.Id, source.TenantId, source.AccountId, false)
		return err
	}
	status := row["status"].String()
	if status == sysin.CollectHistoryTaskStatusPending || status == sysin.CollectHistoryTaskStatusRunning {
		return nil
	}
	taskId := row["id"].Int64()
	if taskId <= 0 {
		return nil
	}
	if err = updateCollectHistoryTask(ctx, taskId, g.Map{
		"status":        sysin.CollectHistoryTaskStatusPending,
		"error_message": "",
		"next_run_at":   nil,
		"finished_at":   nil,
		"updated_at":    gtime.Now(),
	}); err != nil {
		return err
	}
	s.appendCollectHistoryLog(ctx, taskId, source.TenantId, source.AccountId, "info", "retry", "采集源重新开启，历史采集任务已重新投递", nil)
	return s.enqueueCollectHistoryTask(ctx, taskId, time.Second)
}
