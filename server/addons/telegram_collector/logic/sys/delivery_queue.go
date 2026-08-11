package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

const deliveryTaskType = "telegram_collector:delivery"

const (
	deliveryLeaseDuration     = 3 * time.Minute
	deliveryRecoveryInterval  = 30 * time.Second
	deliveryRecoveryStaleTime = 15 * time.Second
	deliveryMaxAttempts       = 5
)

type deliveryTaskPayload struct {
	DeliveryID int64 `json:"deliveryId"`
	Priority   int   `json:"priority"`
}

func (s *sCollector) StartDeliveryRuntime(ctx context.Context) {
	if !collectorEnabled(ctx) {
		g.Log().Info(ctx, "Telegram采集插件处于关闭状态，Publish Worker不会消费采集交付")
		return
	}
	s.queueMu.Lock()
	if s.deliveryServer != nil {
		s.queueMu.Unlock()
		return
	}
	runtimeCtx := s.ensureRuntimeContext(ctx)
	server := asynq.NewServer(collectorRedisOption(runtimeCtx), asynq.Config{
		Concurrency: collectorDeliveryConcurrency(runtimeCtx),
		Queues: map[string]int{
			consts.QueueDeliveryReady: 1,
		},
	})
	if s.queueClient == nil {
		s.queueClient = asynq.NewClient(collectorRedisOption(runtimeCtx))
	}
	s.deliveryServer = server
	s.queueMu.Unlock()

	mux := asynq.NewServeMux()
	mux.HandleFunc(deliveryTaskType, s.handleDeliveryTask)
	g.Log().Infof(runtimeCtx, "启动Telegram Delivery Worker concurrency:%d", collectorDeliveryConcurrency(runtimeCtx))
	s.runtimeWG.Add(2)
	go func() {
		defer s.runtimeWG.Done()
		if err := server.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(runtimeCtx, "Telegram Delivery Worker停止：%+v", err)
		}
	}()
	go func() {
		defer s.runtimeWG.Done()
		s.runDeliveryRecovery(runtimeCtx)
	}()
}

func (s *sCollector) enqueueDeliveryTask(ctx context.Context, deliveryID int64, priority int) error {
	if deliveryID <= 0 {
		return gerror.New("Telegram采集交付任务ID无效")
	}
	body, err := json.Marshal(deliveryTaskPayload{DeliveryID: deliveryID, Priority: priority})
	if err != nil {
		return err
	}
	s.queueMu.Lock()
	if s.queueClient == nil {
		s.queueClient = asynq.NewClient(collectorRedisOption(ctx))
	}
	client := s.queueClient
	s.queueMu.Unlock()
	_, err = client.EnqueueContext(ctx, asynq.NewTask(deliveryTaskType, body),
		asynq.Queue(consts.QueueDeliveryReady),
		asynq.MaxRetry(deliveryMaxAttempts-1),
		asynq.Timeout(3*time.Minute),
	)
	return err
}

func (s *sCollector) handleDeliveryTask(ctx context.Context, task *asynq.Task) error {
	var payload deliveryTaskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return gerror.Wrap(err, "解析Telegram采集交付任务失败")
	}
	startedAt := time.Now()
	if err := s.processDelivery(ctx, payload.DeliveryID); err != nil {
		counter, _ := collectorMeter.Int64Counter("telegram_collector_delivery_total")
		counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", "failed")))
		return err
	}
	histogram, _ := collectorMeter.Float64Histogram("telegram_collector_delivery_duration_seconds")
	histogram.Record(ctx, time.Since(startedAt).Seconds(), metric.WithAttributes(attribute.String("result", "completed")))
	return nil
}

func (s *sCollector) processDelivery(ctx context.Context, deliveryID int64) error {
	if deliveryID <= 0 {
		return gerror.New("Telegram采集交付任务无效")
	}
	columns := dao.TgCollectorDelivery.Columns()
	now := gtime.Now()
	result, err := dao.TgCollectorDelivery.Ctx(ctx).
		WherePri(deliveryID).
		WhereIn(columns.Status, []string{sysin.DeliveryStatusPending, sysin.DeliveryStatusFailedRetry}).
		Data(g.Map{
			columns.Status:       sysin.DeliveryStatusProcessing,
			columns.LeaseOwner:   collectorInstanceID(),
			columns.LeaseUntil:   now.Add(deliveryLeaseDuration),
			columns.AttemptCount: gdb.Raw(columns.AttemptCount + "+1"),
			columns.UpdatedAt:    now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "领取Telegram采集交付失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	var row entity.TgCollectorDelivery
	if err = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Scan(&row); err != nil {
		return s.failDelivery(ctx, deliveryID, err)
	}
	if row.Payload == nil {
		return s.failDelivery(ctx, deliveryID, gerror.New("Telegram采集交付内容为空"))
	}
	var delivery sysin.CollectorDelivery
	if err = json.Unmarshal(row.Payload.MustToJson(), &delivery); err != nil {
		return s.failDelivery(ctx, deliveryID, gerror.Wrap(err, "解析Telegram采集交付失败"))
	}
	delivery.ID = deliveryID
	handler := collectorservice.CollectorDeliveryHandler()
	if handler == nil {
		return s.failDelivery(ctx, deliveryID, gerror.New("Telegram采集交付处理器尚未注册"))
	}
	if err = handler.HandleCollectorDelivery(ctx, &delivery); err != nil {
		return s.failDelivery(ctx, deliveryID, err)
	}
	_, err = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Data(g.Map{
		columns.Status:       sysin.DeliveryStatusCompleted,
		columns.NextRunAt:    nil,
		columns.LeaseOwner:   "",
		columns.LeaseUntil:   nil,
		columns.ErrorMessage: "",
		columns.UpdatedAt:    gtime.Now(),
	}).Update()
	return err
}

func (s *sCollector) failDelivery(ctx context.Context, deliveryID int64, cause error) error {
	columns := dao.TgCollectorDelivery.Columns()
	var row entity.TgCollectorDelivery
	if err := dao.TgCollectorDelivery.Ctx(ctx).Fields(columns.AttemptCount).WherePri(deliveryID).Scan(&row); err != nil {
		return gerror.Wrap(err, "读取Telegram采集交付重试次数失败")
	}
	status := sysin.DeliveryStatusFailedRetry
	var nextRunAt any = gtime.Now().Add(30 * time.Second)
	if row.AttemptCount >= deliveryMaxAttempts {
		status = sysin.DeliveryStatusDead
		nextRunAt = nil
	}
	_, _ = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Data(g.Map{
		columns.Status:       status,
		columns.NextRunAt:    nextRunAt,
		columns.LeaseOwner:   "",
		columns.LeaseUntil:   nil,
		columns.ErrorMessage: cause.Error(),
		columns.UpdatedAt:    gtime.Now(),
	}).Update()
	if status == sysin.DeliveryStatusDead {
		return fmt.Errorf("%w: Telegram采集交付超过最大重试次数: %v", asynq.SkipRetry, cause)
	}
	return cause
}

func (s *sCollector) runDeliveryRecovery(ctx context.Context) {
	timer := time.NewTimer(8 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	s.recoverDeliveryTasks(ctx)
	ticker := time.NewTicker(deliveryRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverDeliveryTasks(ctx)
		}
	}
}

func (s *sCollector) recoverDeliveryTasks(ctx context.Context) {
	limit := collectorRecoveryBatchSize(ctx)
	if err := s.resetExpiredDeliveryLeases(ctx, limit); err != nil {
		g.Log().Warningf(ctx, "恢复超时Telegram采集交付失败：%+v", err)
	}
	if err := s.enqueueDueDeliveryTasks(ctx, limit); err != nil {
		g.Log().Warningf(ctx, "重新投递Telegram采集交付失败：%+v", err)
	}
}

func (s *sCollector) resetExpiredDeliveryLeases(ctx context.Context, limit int) error {
	columns := dao.TgCollectorDelivery.Columns()
	now := gtime.Now()
	rows, err := dao.TgCollectorDelivery.Ctx(ctx).
		Fields(columns.Id, columns.AttemptCount).
		Where(columns.Status, sysin.DeliveryStatusProcessing).
		WhereNotNull(columns.LeaseUntil).
		WhereLTE(columns.LeaseUntil, now).
		OrderAsc(columns.LeaseUntil).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取超时Telegram采集交付失败")
	}
	for _, row := range rows {
		deliveryID := row[columns.Id].Int64()
		if deliveryID <= 0 {
			continue
		}
		status := sysin.DeliveryStatusFailedRetry
		var nextRunAt any = now
		if row[columns.AttemptCount].Int() >= deliveryMaxAttempts {
			status = sysin.DeliveryStatusDead
			nextRunAt = nil
		}
		_, updateErr := dao.TgCollectorDelivery.Ctx(ctx).
			WherePri(deliveryID).
			Where(columns.Status, sysin.DeliveryStatusProcessing).
			WhereLTE(columns.LeaseUntil, now).
			Data(g.Map{
				columns.Status:       status,
				columns.NextRunAt:    nextRunAt,
				columns.LeaseOwner:   "",
				columns.LeaseUntil:   nil,
				columns.ErrorMessage: "处理租约超时，已自动恢复",
				columns.UpdatedAt:    now,
			}).Update()
		if updateErr != nil {
			return gerror.Wrap(updateErr, "重置超时Telegram采集交付失败")
		}
	}
	return nil
}

func (s *sCollector) enqueueDueDeliveryTasks(ctx context.Context, limit int) error {
	columns := dao.TgCollectorDelivery.Columns()
	now := gtime.Now()
	staleBefore := now.Add(-deliveryRecoveryStaleTime)
	condition := fmt.Sprintf(
		"(%s=? AND %s<=?) OR (%s=? AND %s IS NOT NULL AND %s<=?)",
		columns.Status,
		columns.UpdatedAt,
		columns.Status,
		columns.NextRunAt,
		columns.NextRunAt,
	)
	rows, err := dao.TgCollectorDelivery.Ctx(ctx).
		Fields(columns.Id, columns.Priority, columns.Status).
		Where(condition,
			sysin.DeliveryStatusPending, staleBefore,
			sysin.DeliveryStatusFailedRetry, now,
		).
		WhereLT(columns.AttemptCount, deliveryMaxAttempts).
		OrderDesc(columns.Priority).
		OrderAsc(columns.UpdatedAt).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复Telegram采集交付失败")
	}
	for _, row := range rows {
		deliveryID := row[columns.Id].Int64()
		status := row[columns.Status].String()
		if deliveryID <= 0 || status == "" {
			continue
		}
		claim := dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Where(columns.Status, status)
		if status == sysin.DeliveryStatusPending {
			claim = claim.WhereLTE(columns.UpdatedAt, staleBefore)
		} else {
			claim = claim.WhereNotNull(columns.NextRunAt).WhereLTE(columns.NextRunAt, now)
		}
		result, updateErr := claim.Data(g.Map{columns.UpdatedAt: now}).Update()
		if updateErr != nil {
			return gerror.Wrap(updateErr, "领取待恢复Telegram采集交付失败")
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			continue
		}
		if updateErr = s.enqueueDeliveryTask(ctx, deliveryID, row[columns.Priority].Int()); updateErr != nil {
			return gerror.Wrap(updateErr, "投递待恢复Telegram采集交付失败")
		}
	}
	return nil
}

func collectorDeliveryConcurrency(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "telegramCollector.delivery.concurrency", 8).Int()
	if value < 1 {
		return 1
	}
	return value
}
