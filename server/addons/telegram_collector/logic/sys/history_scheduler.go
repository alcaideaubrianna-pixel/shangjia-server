package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

// StartHistoryScheduler starts the compatibility scheduler used by the
// background-worker role. Old deliveries are resolved through their event at
// dispatch time, so a full-table metadata backfill is not required.
func (s *sCollector) StartHistoryScheduler(ctx context.Context) {
	if !g.Cfg().MustGet(ctx, "telegramCollector.delivery.schedulerEnabled", false).Bool() {
		g.Log().Info(ctx, "历史采集调度器未启用")
		return
	}
	s.queueMu.Lock()
	if s.historySchedulerStop != nil {
		s.queueMu.Unlock()
		return
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	s.historySchedulerStop = cancel
	s.queueMu.Unlock()

	go func() {
		ticker := time.NewTicker(historySchedulerInterval(runtimeCtx))
		defer ticker.Stop()
		for {
			select {
			case <-runtimeCtx.Done():
				return
			case <-ticker.C:
				if err := s.dispatchHistoryBatch(runtimeCtx); err != nil {
					g.Log().Warningf(runtimeCtx, "历史采集调度失败：%+v", err)
				}
			}
		}
	}()
	g.Log().Infof(runtimeCtx, "历史采集调度器已启动 batch:%d", historySchedulerBatchSize(runtimeCtx))
}

func (s *sCollector) StopHistoryScheduler() {
	s.queueMu.Lock()
	stop := s.historySchedulerStop
	s.historySchedulerStop = nil
	s.queueMu.Unlock()
	if stop != nil {
		stop()
	}
}

func (s *sCollector) dispatchHistoryBatch(ctx context.Context) error {
	columns := dao.TgCollectorDelivery.Columns()
	owner := collectorInstanceID()
	leaseUntil := gtime.Now().Add(historySchedulerLeaseDuration(ctx))
	rows, err := dao.TgCollectorDelivery.Ctx(ctx).
		Fields("hg_tg_collector_delivery.id", "hg_tg_collector_delivery.priority", "e.account_id", "e.source_id").
		LeftJoin("hg_tg_collector_event e", "e.id = hg_tg_collector_delivery.event_id").
		Where(columns.Status, sysin.DeliveryStatusPending).
		Where("e.source_id > 0").
		Order(columns.Priority + " DESC, " + columns.CreatedAt + " ASC, " + columns.Id + " ASC").
		Limit(historySchedulerBatchSize(ctx) * 4).All()
	if err != nil {
		return err
	}
	accountCounts := make(map[int64]int)
	sourceCounts := make(map[int64]int)
	accountLimit := historySchedulerAccountLimit(ctx)
	sourceLimit := historySchedulerSourceLimit(ctx)
	selected := 0
	for _, row := range rows {
		if selected >= historySchedulerBatchSize(ctx) {
			break
		}
		id := row[columns.Id].Int64()
		if id <= 0 {
			continue
		}
		accountID := row["account_id"].Int64()
		sourceID := row["source_id"].Int64()
		if accountID <= 0 || sourceID <= 0 || accountCounts[accountID] >= accountLimit || sourceCounts[sourceID] >= sourceLimit {
			continue
		}
		priority := row[columns.Priority].Int()
		claimed, err := dao.TgCollectorDelivery.Ctx(ctx).WherePri(id).
			Where(columns.Status, sysin.DeliveryStatusPending).
			Where(fmt.Sprintf("(%s IS NULL OR %s <= ?)", "dispatch_until", "dispatch_until"), gtime.Now()).
			Data(g.Map{"dispatch_owner": owner, "dispatch_until": leaseUntil}).Update()
		if err != nil {
			return err
		}
		if affected, _ := claimed.RowsAffected(); affected == 0 {
			continue
		}
		if err := s.enqueueDeliveryTask(ctx, id, priority); err != nil {
			_, _ = dao.TgCollectorDelivery.Ctx(ctx).WherePri(id).
				Where("dispatch_owner", owner).
				Data(g.Map{"dispatch_owner": "", "dispatch_until": nil}).Update()
			return err
		}
		_, _ = dao.TgCollectorDelivery.Ctx(ctx).WherePri(id).
			Where(columns.Status, sysin.DeliveryStatusPending).
			Data(g.Map{"dispatch_at": gtime.Now(), "dispatch_owner": "", "dispatch_until": nil}).Update()
		accountCounts[accountID]++
		sourceCounts[sourceID]++
		selected++
	}
	return nil
}

func historySchedulerAccountLimit(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.accountConcurrency", 8).Int()
	if value < 1 {
		value = 1
	}
	if value > 128 {
		value = 128
	}
	return value
}

func historySchedulerSourceLimit(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.sourceConcurrency", 4).Int()
	if value < 1 {
		value = 1
	}
	if value > 64 {
		value = 64
	}
	return value
}

func historySchedulerLeaseDuration(ctx context.Context) time.Duration {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.schedulerLeaseSeconds", 60).Int()
	if value < 10 {
		value = 10
	}
	return time.Duration(value) * time.Second
}

func historySchedulerInterval(ctx context.Context) time.Duration {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.schedulerIntervalMilliseconds", 1000).Int()
	if value < 200 {
		value = 200
	}
	return time.Duration(value) * time.Millisecond
}

func historySchedulerBatchSize(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.schedulerBatchSize", 100).Int()
	if value < 1 {
		value = 1
	}
	if value > 1000 {
		value = 1000
	}
	return value
}
