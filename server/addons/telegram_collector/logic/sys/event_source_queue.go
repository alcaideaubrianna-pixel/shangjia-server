package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/collectobserve"
	"hotgo/utility/sourcequeue"
)

const collectorEventSourceNamespace = sourcequeue.CollectorEventNamespace

type collectorSourceScopeKey struct{}
type collectorSourceLaneKey struct{}

func withCollectorSourceScope(ctx context.Context, tenantID, sourceID int64) context.Context {
	return context.WithValue(ctx, collectorSourceScopeKey{}, sourcequeue.Scope{TenantID: tenantID, SourceID: sourceID})
}

func collectorSourceScopeFromContext(ctx context.Context) (sourcequeue.Scope, bool) {
	scope, ok := ctx.Value(collectorSourceScopeKey{}).(sourcequeue.Scope)
	return scope, ok && scope.TenantID > 0 && scope.SourceID > 0
}

func withCollectorSourceLane(ctx context.Context, lane sourcequeue.Lane) context.Context {
	return context.WithValue(ctx, collectorSourceLaneKey{}, lane)
}

func collectorSourceLaneFromContext(ctx context.Context) (sourcequeue.Lane, bool) {
	lane, ok := ctx.Value(collectorSourceLaneKey{}).(sourcequeue.Lane)
	return lane, ok && (lane == sourcequeue.Realtime || lane == sourcequeue.History)
}

// Keep lane identity independent of the persisted event's original priority.
func collectorEventSourceLane(ctx context.Context, priority int, origin eventEnqueueOrigin) sourcequeue.Lane {
	if lane, ok := collectorSourceLaneFromContext(ctx); ok && lane == sourcequeue.History {
		return sourcequeue.History
	}
	if origin != eventOriginIngress || priority < sysin.EventPriorityRealtime {
		return sourcequeue.History
	}
	return sourcequeue.Realtime
}

type collectorSourceEventPayload struct {
	Carrier  collectobserve.Carrier `json:"carrier"`
	EventID  int64                  `json:"eventId"`
	TenantID int64                  `json:"tenantId"`
	SourceID int64                  `json:"sourceId"`
}

func (s *sCollector) enqueueSourceEvent(ctx context.Context, eventID int64, priority int, origin eventEnqueueOrigin) error {
	scope, ok := collectorSourceScopeFromContext(ctx)
	if !ok {
		// Only old callers without an ingress scope need this primary-key lookup.
		c := dao.TgCollectorEvent.Columns()
		row, err := dao.TgCollectorEvent.Ctx(ctx).Fields(c.TenantId, c.SourceId).WherePri(eventID).One()
		if err != nil {
			return gerror.Wrap(err, "读取旧采集事件来源失败")
		}
		if len(row) == 0 {
			return gerror.New("采集事件不存在，不能确认交付")
		}
		scope = sourcequeue.Scope{TenantID: row[c.TenantId].Int64(), SourceID: row[c.SourceId].Int64()}
	}
	if scope.TenantID <= 0 || scope.SourceID <= 0 {
		return gerror.New("采集事件必须指定租户及来源")
	}
	lane := collectorEventSourceLane(ctx, priority, origin)
	payload, err := json.Marshal(collectorSourceEventPayload{EventID: eventID, TenantID: scope.TenantID, SourceID: scope.SourceID, Carrier: collectobserve.Capture(ctx, time.Now())})
	if err != nil {
		return gerror.Wrap(err, "编码来源采集事件失败")
	}
	q, err := sourcequeue.New(collectorEventSourceNamespace)
	if err != nil {
		return gerror.Wrap(err, "初始化来源采集队列失败")
	}
	started := time.Now()
	if origin == eventOriginRecovery || origin == eventOriginReplay {
		for _, existingLane := range []sourcequeue.Lane{sourcequeue.Realtime, sourcequeue.History} {
			exists, lookupErr := q.HasTask(ctx, scope, existingLane, strconv.FormatInt(eventID, 10))
			if lookupErr != nil {
				return gerror.Wrap(lookupErr, "检查来源采集任务失败，禁止降级重复投递")
			}
			if exists {
				logCollectorPipelineStage(ctx, &entity.TgCollectorEvent{Id: eventID, TenantId: scope.TenantID, SourceId: scope.SourceID}, "event.recovery.existing_task_preserved", started, nil, nil, g.Map{"outcome": "existing_task_preserved", "queueLane": existingLane, "enqueueOrigin": origin})
				return nil
			}
		}
	}
	err = q.Enqueue(ctx, scope, lane, strconv.FormatInt(eventID, 10), payload)
	logCollectorPipelineStage(ctx, &entity.TgCollectorEvent{Id: eventID, TenantId: scope.TenantID, SourceId: scope.SourceID}, "event.enqueued", started, nil, err,
		g.Map{"queueAdapter": "cache.Instance/sourcequeue", "queueLane": lane, "enqueueOrigin": origin})
	return err
}

// Legacy Asynq is a migration reader only; all business work uses source lanes.
func (s *sCollector) migrateLegacySourceEvent(ctx context.Context, eventID int64) error {
	return s.enqueueSourceEvent(withCollectorSourceLane(ctx, sourcequeue.History), eventID, sysin.EventPriorityNormal, eventOriginRecovery)
}

type collectorEventSourceQueue interface {
	Claim(context.Context, sourcequeue.Lane, time.Duration) (*sourcequeue.Claim, error)
	Ack(context.Context, *sourcequeue.Claim) error
	Nack(context.Context, *sourcequeue.Claim, time.Duration) error
	Renew(context.Context, *sourcequeue.Claim, time.Duration) error
}

func collectorSourceQueuePause(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *sCollector) runSourceEventWorker(ctx context.Context, q collectorEventSourceQueue, lane sourcequeue.Lane) {
	for ctx.Err() == nil {
		claim, err := q.Claim(ctx, lane, eventLeaseDuration)
		if err != nil {
			g.Log().Warning(ctx, "来源采集队列领取失败", g.Map{"event": "collector.event.source_claim_failed", "lane": lane, "error": err.Error()})
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
		err = executeSourceEventClaim(ctx, q, claim, lane, s.processEvent)
		if err != nil {
			g.Log().Warning(ctx, "来源采集任务未确认，等待重试", g.Map{"event": "collector.event.source_unconfirmed", "tenantId": claim.Scope.TenantID, "sourceId": claim.Scope.SourceID, "eventId": claim.ID, "lane": lane, "error": err.Error()})
			if !collectorSourceQueuePause(ctx, time.Second) {
				return
			}
		}
	}
}

func executeSourceEventClaim(ctx context.Context, q collectorEventSourceQueue, claim *sourcequeue.Claim, lane sourcequeue.Lane, process func(context.Context, int64) error) error {
	if claim == nil {
		return gerror.New("来源采集领取记录为空")
	}
	var payload collectorSourceEventPayload
	if err := json.Unmarshal(claim.Payload, &payload); err != nil {
		return gerror.Wrap(err, "来源采集任务格式无效")
	}
	if claim.Token == "" || claim.Lane != lane || payload.EventID <= 0 || claim.ID != strconv.FormatInt(payload.EventID, 10) ||
		claim.Scope.TenantID <= 0 || claim.Scope.SourceID <= 0 || payload.TenantID != claim.Scope.TenantID || payload.SourceID != claim.Scope.SourceID {
		return gerror.New("来源采集任务归属不一致，禁止确认")
	}
	now := time.Now()
	ctx = payload.Carrier.RestoreAt(ctx, now)
	fields := g.Map{"queueAdapter": "sourcequeue", "queueLane": lane, "tenantId": payload.TenantID, "sourceId": payload.SourceID, "eventId": payload.EventID, "queueAgeAvailable": false}
	if enqueuedAt := payload.Carrier.EnqueuedAtUnixNano; enqueuedAt > 0 && enqueuedAt <= now.UnixNano() {
		fields["queueAgeAvailable"] = true
		fields["queueAgeMs"] = now.Sub(time.Unix(0, enqueuedAt)).Milliseconds()
	}
	logCollectorPipelineStage(ctx, &entity.TgCollectorEvent{Id: payload.EventID, TenantId: payload.TenantID, SourceId: payload.SourceID}, "event.source.claimed", now, nil, nil, fields)
	workCtx, cancel := context.WithTimeout(withCollectorSourceLane(withCollectorSourceScope(ctx, payload.TenantID, payload.SourceID), lane), eventLeaseDuration)
	defer cancel()
	stopRenew, renewed := make(chan struct{}), make(chan error, 1)
	go func() {
		ticker := time.NewTicker(eventLeaseDuration / 3)
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
				renewCtx, stop := context.WithTimeout(workCtx, 5*time.Second)
				err := q.Renew(renewCtx, claim, eventLeaseDuration)
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
				processErr = gerror.Newf("来源采集处理异常：%v", recovered)
			}
		}()
		processErr = process(workCtx, payload.EventID)
	}()
	close(stopRenew)
	renewErr := <-renewed
	if renewErr != nil || workCtx.Err() != nil {
		return errors.Join(processErr, renewErr, workCtx.Err())
	}
	finishCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	if processErr != nil {
		// A durable dead Event can be ACKed; lease/Redis errors never can.
		if errors.Is(processErr, asynq.SkipRetry) {
			return q.Ack(finishCtx, claim)
		}
		return errors.Join(processErr, q.Nack(finishCtx, claim, 30*time.Second))
	}
	return q.Ack(finishCtx, claim)
}

// Claim and RETURNING remain a single indexed round trip on the hot path.
// The scope predicate fences a corrupted queue payload before changing any row.
func claimSourceCollectorEvent(ctx context.Context, eventID int64, owner string, scope sourcequeue.Scope) (entity.TgCollectorEvent, error) {
	c := dao.TgCollectorEvent.Columns()
	if eventID <= 0 || owner == "" || scope.TenantID <= 0 || scope.SourceID <= 0 {
		return entity.TgCollectorEvent{}, gerror.New("采集事件领取归属无效")
	}
	now := "(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai')"
	query := fmt.Sprintf(`UPDATE %s SET %s='%s',%s=?,%s=%s+INTERVAL '%d seconds',%s=%s+1,%s=%s
 WHERE %s=? AND %s=? AND %s=? AND %s IN ('%s','%s') AND (%s IS NULL OR %s<=%s) RETURNING *`,
		dao.TgCollectorEvent.Table(), c.Status, sysin.EventStatusProcessing, c.LeaseOwner, c.LeaseUntil, now, int64(eventLeaseDuration/time.Second), c.AttemptCount, c.AttemptCount, c.UpdatedAt, now,
		c.Id, c.TenantId, c.SourceId, c.Status, sysin.EventStatusReceived, sysin.EventStatusFailedRetry, c.NextRunAt, c.NextRunAt, now)
	rows, err := g.DB().GetAll(ctx, query, owner, eventID, scope.TenantID, scope.SourceID)
	if err != nil {
		return entity.TgCollectorEvent{}, gerror.Wrap(err, "领取来源采集事件失败")
	}
	if len(rows) == 1 {
		var row entity.TgCollectorEvent
		if err = rows[0].Struct(&row); err != nil {
			return row, gerror.Wrap(err, "解析采集事件失败")
		}
		return row, nil
	}
	// Only contention, delayed retry, terminal receipts or a scope mismatch enter
	// this primary-key check. Never acknowledge work owned by another DB lease.
	row, err := dao.TgCollectorEvent.Ctx(ctx).Fields(c.TenantId, c.SourceId, c.Status).WherePri(eventID).One()
	if err != nil {
		return entity.TgCollectorEvent{}, gerror.Wrap(err, "确认采集事件领取状态失败")
	}
	if len(row) == 0 || row[c.TenantId].Int64() != scope.TenantID || row[c.SourceId].Int64() != scope.SourceID {
		return entity.TgCollectorEvent{}, gerror.New("持久化采集事件与队列来源不一致")
	}
	switch row[c.Status].String() {
	case sysin.EventStatusReady, sysin.EventStatusDead, sysin.EventStatusIgnored:
		return entity.TgCollectorEvent{}, nil
	default:
		return entity.TgCollectorEvent{}, gerror.New("采集事件仍待执行或租约被占用，保留队列任务")
	}
}

func (s *sCollector) runSourceEventRecovery(ctx context.Context, q *sourcequeue.Queue) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, lane := range []sourcequeue.Lane{sourcequeue.Realtime, sourcequeue.History} {
				recoveryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				err := q.Recover(recoveryCtx, lane, 50)
				cancel()
				if err != nil {
					g.Log().Warning(ctx, "恢复来源采集队列失败，不回退数据库扫描", g.Map{"event": "collector.event.source_recovery_failed", "lane": lane, "error": err.Error()})
					break
				}
			}
		}
	}
}
