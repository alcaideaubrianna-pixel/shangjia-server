package sys

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/collectobserve"
	"hotgo/utility/sourcequeue"
)

const collectorDeliverySourceNamespace = sourcequeue.CollectorDeliveryNamespace

// This key is set only by a verified Delivery claim, not by a raw Event handoff.
type deliverySourceClaimKey struct{}
type deliverySourceRecoveryKey struct{}

type deliverySourcePayload struct {
	DeliveryID int64  `json:"deliveryId"`
	TenantID   int64  `json:"tenantId"`
	SourceID   int64  `json:"sourceId"`
	Carrier    []byte `json:"carrier,omitempty"`
}

type deliverySourceQueue interface {
	Claim(context.Context, sourcequeue.Lane, time.Duration) (*sourcequeue.Claim, error)
	Ack(context.Context, *sourcequeue.Claim) error
	Nack(context.Context, *sourcequeue.Claim, time.Duration) error
	Renew(context.Context, *sourcequeue.Claim, time.Duration) error
	Recover(context.Context, sourcequeue.Lane, int) error
	HasImmediateWork(context.Context, sourcequeue.Scope, sourcequeue.Lane) (bool, error)
}

func deliverySourceHistoryBudgets(total int) (workers, legacy int) {
	if total <= 0 {
		return 0, 0
	}
	if total == 1 {
		return 1, 0
	}
	return total - 1, 1
}

func deliverySourceLane(ctx context.Context, priority int) sourcequeue.Lane {
	if lane, ok := collectorSourceLaneFromContext(ctx); ok {
		return lane
	}
	if priority < sysin.EventPriorityRealtime {
		return sourcequeue.History
	}
	return sourcequeue.Realtime
}

func (s *sCollector) enqueueDeliverySourceTask(ctx context.Context, id int64, priority int) error {
	if id <= 0 {
		return gerror.New("Telegram采集交付任务ID无效")
	}
	scope, ok := collectorSourceScopeFromContext(ctx)
	if !ok {
		// Compatibility/recovery callers without an already loaded scope perform
		// one primary-key lookup, never per-stage route queries or a source scan.
		d, e := dao.TgCollectorDelivery.Columns(), dao.TgCollectorEvent.Columns()
		row, err := dao.TgCollectorDelivery.Ctx(ctx).As("d").
			LeftJoin(dao.TgCollectorEvent.Table()+" e", "e."+e.Id+" = d."+d.EventId).
			Fields("d."+d.TenantId+" AS tenant_id", "e."+e.SourceId+" AS source_id").
			Where("d."+d.Id, id).One()
		if err != nil {
			return gerror.Wrap(err, "读取采集交付来源失败")
		}
		scope = sourcequeue.Scope{TenantID: row["tenant_id"].Int64(), SourceID: row["source_id"].Int64()}
	}
	if scope.TenantID <= 0 || scope.SourceID <= 0 {
		return gerror.New("交付缺少租户或来源，禁止回退共享队列")
	}
	lane := deliverySourceLane(ctx, priority)
	payload, err := json.Marshal(deliverySourcePayload{id, scope.TenantID, scope.SourceID, collectorTaskPayloadForEnqueue(ctx, id, true)})
	if err != nil {
		return gerror.Wrap(err, "编码来源交付任务失败")
	}
	q, err := sourcequeue.New(collectorDeliverySourceNamespace)
	if err != nil {
		return gerror.Wrap(err, "初始化来源交付队列失败")
	}
	ctx = withCollectorSourceLane(withCollectorSourceScope(ctx, scope.TenantID, scope.SourceID), lane)
	// DB recovery observes existing work in either lane without advancing its
	// generation or promoting history. Ingress/replay must still publish new wakeups.
	if recovery, _ := ctx.Value(deliverySourceRecoveryKey{}).(bool); recovery {
		for _, existingLane := range []sourcequeue.Lane{sourcequeue.Realtime, sourcequeue.History} {
			exists, err := q.HasTask(ctx, scope, existingLane, strconv.FormatInt(id, 10))
			if err != nil {
				return err
			}
			if exists {
				logCollectorDeliveryEnqueue(ctx, id, deliverySourceLogicalQueue(existingLane), "already_queued", nil)
				return nil
			}
		}
	}
	err = q.Enqueue(ctx, scope, lane, strconv.FormatInt(id, 10), payload)
	status := "enqueued"
	if err != nil {
		status = "failed"
	}
	logCollectorDeliveryEnqueue(ctx, id, collectorQueueName(ctx), status, err)
	return err
}

// Old history tasks only migrate to the new history queue. A successful ACK
// means Redis accepted the new task, not that business processing completed.
func (s *sCollector) migrateLegacyDeliveryTask(ctx context.Context, task *asynq.Task) error {
	ctx, id, err := collectobserve.DecodeTask(ctx, task.Payload(), time.Now())
	if err != nil {
		return gerror.Wrap(err, "读取历史交付任务失败")
	}
	return s.enqueueDeliverySourceTask(withCollectorSourceLane(ctx, sourcequeue.History), id, sysin.EventPriorityNormal)
}

func (s *sCollector) startDeliverySourcePools(ctx context.Context, q deliverySourceQueue, realtime, history int) {
	for _, pool := range []struct {
		lane    sourcequeue.Lane
		workers int
	}{{sourcequeue.Realtime, realtime}, {sourcequeue.History, history}} {
		lane := pool.lane
		for i := 0; i < pool.workers; i++ {
			s.runtimeWG.Add(1)
			go func() { defer s.runtimeWG.Done(); s.runDeliverySourceWorker(ctx, q, lane) }()
		}
		s.runtimeWG.Add(1)
		go func() { defer s.runtimeWG.Done(); runDeliverySourceRecovery(ctx, q, lane) }()
	}
}

// This recovers only due Redis lease/delay index entries; no PG queries.
func runDeliverySourceRecovery(ctx context.Context, q deliverySourceQueue, lane sourcequeue.Lane) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recoveryCtx, cancel := context.WithTimeout(ctx, time.Second)
			err := q.Recover(recoveryCtx, lane, 64)
			cancel()
			if err != nil && ctx.Err() == nil {
				g.Log().Warning(ctx, "来源交付索引恢复失败", g.Map{"lane": lane, "error": err.Error()})
			}
		}
	}
}

func (s *sCollector) runDeliverySourceWorker(ctx context.Context, q deliverySourceQueue, lane sourcequeue.Lane) {
	for ctx.Err() == nil {
		claim, err := q.Claim(ctx, lane, deliveryLeaseDuration)
		if err != nil {
			g.Log().Warning(ctx, "来源交付队列领取失败", g.Map{"lane": lane, "error": err.Error()})
			if !collectorSourceQueuePause(ctx, time.Second) {
				return
			}
			continue
		}
		if claim == nil {
			if !collectorSourceQueuePause(ctx, 100*time.Millisecond) {
				return
			}
			continue
		}
		lease := deliveryLeaseDuration
		if lane == sourcequeue.History {
			lease = 5 * time.Second
		}
		err = executeDeliverySourceClaim(ctx, q, claim, lane, lease, s.processDelivery)
		if err != nil {
			logCollectorDeliveryStage(ctx, "delivery.source.unconfirmed", 0, 0, "", err, g.Map{"tenantId": claim.Scope.TenantID, "sourceId": claim.Scope.SourceID, "taskId": claim.ID, "lane": lane})
			if !collectorSourceQueuePause(ctx, time.Second) {
				return
			}
		}
	}
}

func validateDeliverySourceScope(scope sourcequeue.Scope, row *entity.TgCollectorDelivery, event *entity.TgCollectorEvent) error {
	if row == nil || event == nil || scope.TenantID <= 0 || scope.SourceID <= 0 || row.TenantId != scope.TenantID || event.TenantId != row.TenantId || event.SourceId != scope.SourceID || event.Id != row.EventId {
		return gerror.New("来源交付领取与持久事件归属不一致，禁止执行")
	}
	return nil
}

// processDelivery loads its existing row/event and validates this claim before
// invoking the handler. No extra route read is added to processing stages.
func executeDeliverySourceClaim(ctx context.Context, q deliverySourceQueue, claim *sourcequeue.Claim, lane sourcequeue.Lane, lease time.Duration, process func(context.Context, int64, ...time.Time) error) error {
	if claim == nil {
		return gerror.New("来源交付领取为空")
	}
	var payload deliverySourcePayload
	if err := json.Unmarshal(claim.Payload, &payload); err != nil {
		return gerror.Wrap(err, "来源交付任务格式无效")
	}
	if claim.Token == "" || claim.Lane != lane || payload.DeliveryID <= 0 || claim.ID != strconv.FormatInt(payload.DeliveryID, 10) || claim.Scope.TenantID <= 0 || claim.Scope.SourceID <= 0 || payload.TenantID != claim.Scope.TenantID || payload.SourceID != claim.Scope.SourceID {
		return gerror.New("来源交付领取与任务归属不一致，禁止执行")
	}
	if lane == sourcequeue.History {
		busy, err := q.HasImmediateWork(ctx, claim.Scope, sourcequeue.Realtime)
		if err != nil {
			return err
		}
		if busy {
			return q.Nack(ctx, claim, time.Second)
		}
	}
	workCtx := withCollectorSourceLane(withCollectorSourceScope(ctx, claim.Scope.TenantID, claim.Scope.SourceID), lane)
	workCtx = context.WithValue(workCtx, deliverySourceClaimKey{}, claim.Scope)
	if len(payload.Carrier) > 0 {
		restored, id, err := collectobserve.DecodeTask(workCtx, payload.Carrier, time.Now())
		if err != nil || id != payload.DeliveryID {
			return gerror.New("来源交付观测载体任务编号不一致")
		}
		workCtx = restored
	}
	workCtx, cancel := context.WithTimeout(workCtx, lease)
	defer cancel()
	stopRenew, renewed := make(chan struct{}), make(chan error, 1)
	go func() {
		ticker := time.NewTicker(lease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-workCtx.Done():
				renewed <- workCtx.Err()
				return
			case <-stopRenew:
				renewed <- nil
				return
			case <-ticker.C:
				renewCtx, stop := context.WithTimeout(workCtx, min(5*time.Second, lease/3))
				err := q.Renew(renewCtx, claim, lease)
				stop()
				if err != nil {
					cancel()
					renewed <- err
					return
				}
			}
		}
	}()
	var processErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				processErr = gerror.Newf("来源交付处理异常：%v", recovered)
			}
		}()
		processErr = process(workCtx, payload.DeliveryID, time.Now())
	}()
	close(stopRenew)
	renewErr := <-renewed
	// A timed-out/cancelled processor cannot ACK even if it ignored cancellation
	// and returned nil. Settlement always retains the original immutable token.
	if renewErr != nil || workCtx.Err() != nil {
		return errors.Join(processErr, renewErr, workCtx.Err())
	}
	finishCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	if processErr == nil || errors.Is(processErr, asynq.SkipRetry) {
		return q.Ack(finishCtx, claim)
	}
	return errors.Join(processErr, q.Nack(finishCtx, claim, 30*time.Second))
}

// Logical names deliberately match existing business pool classification.
func deliverySourceLogicalQueue(lane sourcequeue.Lane) string {
	if lane == sourcequeue.History {
		return consts.QueueDeliveryHistory
	}
	return consts.QueueDeliveryRealtime
}
