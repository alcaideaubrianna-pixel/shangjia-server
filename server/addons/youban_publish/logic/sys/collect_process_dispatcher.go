package sys

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/hgrds/lock"
)

const collectProcessDispatchInterval = 10 * time.Second

func (s *sSysPublish) runCollectProcessDispatcher(ctx context.Context) {
	s.dispatchCollectProcessOnce(ctx)
	ticker := time.NewTicker(collectProcessDispatchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.collectProcessRefresh:
			s.dispatchCollectProcessOnce(ctx)
		case <-ticker.C:
			s.dispatchCollectProcessOnce(ctx)
		}
	}
}

func (s *sSysPublish) dispatchCollectProcessOnce(ctx context.Context) {
	activeSourceIDs, err := enabledCollectProcessSourceIDs(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取启用采集源失败：%+v", err)
		return
	}
	if len(activeSourceIDs) == 0 {
		return
	}
	activeHistorySourceIDs, err := activeCollectHistorySourceIDs(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取历史采集中的来源失败：%+v", err)
		return
	}
	deadline := gtime.Now().Add(-collectMaterialGroupingDelay)
	mod := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("source_id,tenant_id,account_id").
		WhereIn("source_id", activeSourceIDs).
		WhereIn("status", []string{
			sysin.CollectEventStatusPending,
			sysin.CollectEventStatusGroupCollect,
			sysin.CollectEventStatusWaitingOrder,
			sysin.CollectEventStatusPrechecked,
			sysin.CollectEventStatusMediaPending,
			sysin.CollectEventStatusMediaReady,
			sysin.CollectEventStatusIgnored,
		}).
		Where("processed_at IS NULL OR (status = ? AND material_role = ? AND material_parent_event_id > 0 AND error_message = ?)", sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage).
		Where("(received_at <= ? OR (received_at IS NULL AND created_at <= ?))", deadline, deadline)
	if len(activeHistorySourceIDs) > 0 {
		mod = mod.WhereNotIn("source_id", activeHistorySourceIDs)
	}
	rows, err := mod.
		Group("source_id,tenant_id,account_id").
		Limit(1000).
		All()
	if err != nil {
		g.Log().Warningf(ctx, "读取到期采集窗口失败：%+v", err)
		return
	}
	var wg sync.WaitGroup
	for _, row := range rows {
		payload := collectProcessQueuePayload{
			SourceId:  row["source_id"].Int64(),
			TenantId:  row["tenant_id"].Int64(),
			AccountId: row["account_id"].Int64(),
		}
		if payload.SourceId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
			continue
		}
		wg.Add(1)
		go func(payload collectProcessQueuePayload) {
			defer wg.Done()
			if lockErr := s.processCollectSourceWindowWithLock(ctx, payload); lockErr != nil {
				g.Log().Debugf(ctx, "采集窗口已有实例处理，跳过本轮 sourceId:%d err:%+v", payload.SourceId, lockErr)
			}
		}(payload)
	}
	wg.Wait()
}

func (s *sSysPublish) processCollectSourceWindowWithLock(ctx context.Context, payload collectProcessQueuePayload) error {
	enabled, err := collectProcessSourceEnabled(ctx, payload)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	key := fmt.Sprintf("youban_publish:collect:source:%d:%d:%d", payload.TenantId, payload.AccountId, payload.SourceId)
	distributedLock := lock.NewConfig(15*time.Minute, time.Second).Mutex(key)
	if err = distributedLock.TryLock(ctx); err != nil {
		return err
	}
	defer func() { _ = distributedLock.Unlock(context.Background()) }()
	g.Log().Infof(ctx, "采集窗口开始执行 sourceId:%d tenantId:%d accountId:%d", payload.SourceId, payload.TenantId, payload.AccountId)
	if err := s.processCollectSourceWindow(ctx, payload); err != nil {
		return fmt.Errorf("采集窗口执行失败 sourceId:%d: %w", payload.SourceId, err)
	}
	return nil
}

func enabledCollectProcessSourceIDs(ctx context.Context) ([]int64, error) {
	values, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields("id").
		Where("collect_enabled", 1).
		Where("status", 1).
		WhereNull("deleted_at").
		Array()
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		if id := value.Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	return uniqueIds(ids), nil
}

func collectProcessSourceEnabled(ctx context.Context, payload collectProcessQueuePayload) (bool, error) {
	count, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", payload.SourceId).
		Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).
		Where("collect_enabled", 1).
		Where("status", 1).
		WhereNull("deleted_at").
		Count()
	return count > 0, err
}

func activeCollectHistorySourceIDs(ctx context.Context) ([]int64, error) {
	values, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).
		Fields("source_id").
		WhereIn("status", []string{
			sysin.CollectHistoryTaskStatusPending,
			sysin.CollectHistoryTaskStatusRunning,
			sysin.CollectHistoryTaskStatusPaused,
		}).
		Group("source_id").
		Array()
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(values))
	for _, value := range values {
		if id := value.Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	return uniqueIds(ids), nil
}
