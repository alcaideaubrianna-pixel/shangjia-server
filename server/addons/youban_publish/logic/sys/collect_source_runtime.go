package sys

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"hotgo/utility/sourcequeue"
	"strings"
	"time"
)

const collectSourceRuntimeNamespace = sourcequeue.CollectProcessNamespace

type collectSourceRuntimeKind string

const (
	collectSourceRuntimeProcess collectSourceRuntimeKind = "process"
	collectSourceRuntimeOutbox  collectSourceRuntimeKind = "outbox"
)

type collectSourceRuntimeContextKey struct{}

func collectSourceRuntimeJob(ctx context.Context) bool {
	value, _ := ctx.Value(collectSourceRuntimeContextKey{}).(bool)
	return value
}

type collectSourceRuntimeEnvelope struct {
	Kind             collectSourceRuntimeKind   `json:"kind"`
	Process          collectProcessQueuePayload `json:"process"`
	EnqueuedAtUnixMs int64                      `json:"enqueuedAtUnixMs"`
}

type collectSourceRuntimeQueue interface {
	Claim(context.Context, sourcequeue.Lane, time.Duration) (*sourcequeue.Claim, error)
	Ack(context.Context, *sourcequeue.Claim) error
	Nack(context.Context, *sourcequeue.Claim, time.Duration) error
	Renew(context.Context, *sourcequeue.Claim, time.Duration) error
	Recover(context.Context, sourcequeue.Lane, int) error
}

func collectSourceRuntimeConfig(ctx context.Context, key string, def, low, high int) int {
	return max(low, min(high, g.Cfg().MustGet(ctx, key, def).Int()))
}
func collectSourceRuntimeConcurrency(ctx context.Context, lane sourcequeue.Lane) int {
	if lane == sourcequeue.History {
		return collectSourceRuntimeConfig(ctx, "youbanPublish.queue.collectSourceHistoryConcurrency", 1, 0, 16)
	}
	return collectSourceRuntimeConfig(ctx, "youbanPublish.queue.collectSourceRealtimeConcurrency", 4, 1, 64)
}
func collectSourceRuntimeLease(ctx context.Context) time.Duration {
	return time.Duration(collectSourceRuntimeConfig(ctx, "youbanPublish.collect.sourceRuntimeLeaseSeconds", 30, 10, 300)) * time.Second
}
func collectSourceRuntimePoll(ctx context.Context) time.Duration {
	return time.Duration(collectSourceRuntimeConfig(ctx, "youbanPublish.collect.sourceRuntimePollMilliseconds", 200, 50, 5000)) * time.Millisecond
}
func collectSourceRuntimeLane(payload collectProcessQueuePayload) sourcequeue.Lane {
	if payload.History || payload.Recovery {
		return sourcequeue.History
	}
	return sourcequeue.Realtime
}

// Retain account/chat/event/rule/owner identity, even on event=0 window wakes.
// Trace/time are deliberately excluded; waiting retries keep the same job ID.
func collectSourceRuntimeID(payload collectProcessQueuePayload) (string, error) {
	payload.Observation = nil
	body, err := collectProcessTaskBody(payload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("process:%x", sha256.Sum256(body)), nil
}

func enqueueCollectSourceProcess(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration) error {
	id, err := collectSourceRuntimeID(payload)
	if err != nil {
		return err
	}
	return enqueueCollectSourceEnvelope(ctx, collectSourceRuntimeEnvelope{Kind: collectSourceRuntimeProcess, Process: payload, EnqueuedAtUnixMs: time.Now().UnixMilli()}, collectSourceRuntimeLane(payload), id, delay)
}
func enqueueCollectSourceOutboxWake(ctx context.Context, payload collectProcessQueuePayload) error {
	if payload.TenantId <= 0 || payload.AccountId < 0 || payload.SourceId <= 0 {
		return gerror.New("采集Outbox唤醒归属无效")
	}
	// Wake identity is scope+account; dispatch itself remains source-scoped.
	wake := collectProcessQueuePayload{TenantId: payload.TenantId, AccountId: payload.AccountId, SourceId: payload.SourceId, History: true, Recovery: true}
	return enqueueCollectSourceEnvelope(ctx, collectSourceRuntimeEnvelope{Kind: collectSourceRuntimeOutbox, Process: wake, EnqueuedAtUnixMs: time.Now().UnixMilli()}, sourcequeue.History, fmt.Sprintf("outbox:%d:%d:%d", wake.TenantId, wake.AccountId, wake.SourceId), 0)
}
func enqueueCollectSourceEnvelope(ctx context.Context, envelope collectSourceRuntimeEnvelope, lane sourcequeue.Lane, id string, delay time.Duration) error {
	payload := envelope.Process
	if payload.TenantId <= 0 || payload.AccountId < 0 || payload.SourceId <= 0 {
		return gerror.New("采集来源队列任务归属无效")
	}
	if delay < 0 {
		delay = 0
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return gerror.Wrap(err, "编码采集来源任务失败")
	}
	queue, err := sourcequeue.New(collectSourceRuntimeNamespace)
	if err != nil {
		return err
	}
	started := time.Now()
	scope := sourcequeue.Scope{TenantID: payload.TenantId, SourceID: payload.SourceId}
	if payload.PrioritizeHistory {
		if lane != sourcequeue.History || delay != 0 {
			return gerror.New("历史提优先级任务必须立即进入历史队列")
		}
		err = queue.EnqueueHistoryPriority(ctx, scope, id, body)
	} else {
		err = queue.EnqueueAfter(ctx, scope, lane, id, body, delay)
	}
	fields := collectSourceRuntimeFields(envelope, lane)
	fields["event"] = "collect.source.enqueued"
	fields["enqueueDurationMs"] = time.Since(started).Milliseconds()
	fields["delayMs"] = delay.Milliseconds()
	if err != nil {
		fields["error"] = err.Error()
		g.Log().Warning(ctx, "采集来源队列入队失败", fields)
		return gerror.Wrap(err, "采集来源队列入队失败")
	}
	g.Log().Info(ctx, "采集来源队列入队", fields)
	return nil
}

func collectSourceRuntimeFields(envelope collectSourceRuntimeEnvelope, lane sourcequeue.Lane) g.Map {
	p := envelope.Process
	return g.Map{"adapter": "sourcequeue", "queueLane": lane, "kind": envelope.Kind, "tenantId": p.TenantId, "accountId": p.AccountId, "sourceId": p.SourceId, "sourceChatId": p.SourceChatId, "eventId": p.EventId, "collectEventId": p.EventId, "ruleId": p.RuleId, "ruleQueueKey": collectProcessRuleQueueKey(p)}
}

func decodeCollectSourceRuntimeClaim(claim *sourcequeue.Claim) (collectSourceRuntimeEnvelope, error) {
	var envelope collectSourceRuntimeEnvelope
	if claim == nil {
		return envelope, gerror.New("采集来源任务缺少领取凭据")
	}
	if err := json.Unmarshal(claim.Payload, &envelope); err != nil {
		return envelope, gerror.Wrap(err, "解析采集来源任务失败")
	}
	p := envelope.Process
	if p.TenantId <= 0 || p.AccountId < 0 || p.SourceId <= 0 || claim.Scope.TenantID != p.TenantId || claim.Scope.SourceID != p.SourceId {
		return envelope, gerror.New("采集来源任务与领取范围不一致")
	}
	if envelope.Kind != collectSourceRuntimeProcess && envelope.Kind != collectSourceRuntimeOutbox {
		return envelope, gerror.New("采集来源任务类型不合法")
	}
	if envelope.Kind == collectSourceRuntimeProcess && claim.Lane != collectSourceRuntimeLane(p) {
		return envelope, gerror.New("采集来源任务通道不一致")
	}
	if envelope.Kind == collectSourceRuntimeOutbox && claim.Lane != sourcequeue.History {
		return envelope, gerror.New("采集Outbox唤醒必须使用历史预算")
	}
	return envelope, nil
}

func (s *sSysPublish) runCollectSourceRuntimeWorker(ctx context.Context, queue collectSourceRuntimeQueue, lane sourcequeue.Lane) {
	lease, poll := collectSourceRuntimeLease(ctx), collectSourceRuntimePoll(ctx)
	for ctx.Err() == nil {
		started := time.Now()
		claimCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		claim, err := queue.Claim(claimCtx, lane, lease)
		cancel()
		if err != nil {
			g.Log().Warning(ctx, "领取采集来源任务失败", g.Map{"event": "collect.source.claim.failed", "adapter": "sourcequeue", "queueLane": lane, "waitReason": "queue_unavailable", "error": err.Error()})
			if !waitCollectSourceRuntime(ctx, time.Second) {
				return
			}
			continue
		}
		if claim == nil {
			if !waitCollectSourceRuntime(ctx, poll) {
				return
			}
			continue
		}
		s.executeCollectSourceRuntimeClaim(ctx, queue, claim, lease, time.Since(started))
	}
}

func (s *sSysPublish) executeCollectSourceRuntimeClaim(ctx context.Context, queue collectSourceRuntimeQueue, claim *sourcequeue.Claim, lease, claimDuration time.Duration) {
	executeCollectSourceRuntimeClaimWithHandler(ctx, queue, claim, lease, claimDuration, s.handleCollectSourceRuntimeEnvelope)
}

func (s *sSysPublish) handleCollectSourceRuntimeEnvelope(ctx context.Context, envelope collectSourceRuntimeEnvelope) error {
	ctx = context.WithValue(ctx, collectSourceRuntimeContextKey{}, true)
	if envelope.Kind == collectSourceRuntimeOutbox {
		return s.executeCollectSourceOutboxWake(ctx, envelope.Process)
	}
	body, err := collectProcessTaskBody(envelope.Process)
	if err != nil {
		return err
	}
	return s.handleCollectProcessTask(ctx, asynq.NewTask(tgTaskTypeCollectProcess, body))
}

// Each lane already owns a bounded worker pool and lease. A nonempty realtime
// queue must not veto the reserved history capacity. Actual event/window locks
// and dependency waits remain inside the business handler and retain retry.
func executeCollectSourceRuntimeClaimWithHandler(ctx context.Context, queue collectSourceRuntimeQueue, claim *sourcequeue.Claim, lease, claimDuration time.Duration, handle func(context.Context, collectSourceRuntimeEnvelope) error) {
	envelope, decodeErr := decodeCollectSourceRuntimeClaim(claim)
	fields := collectSourceRuntimeFields(envelope, claim.Lane)
	fields["event"] = "collect.source.claimed"
	fields["claimDurationMs"] = claimDuration.Milliseconds()
	fields["queueAgeAvailable"] = envelope.EnqueuedAtUnixMs > 0 && envelope.EnqueuedAtUnixMs <= time.Now().UnixMilli()
	if fields["queueAgeAvailable"] == true {
		fields["queueAgeMs"] = time.Now().UnixMilli() - envelope.EnqueuedAtUnixMs
	}
	// Keep the original observation carrier; queue timing has its own envelope.
	ctx = restoreCollectProcessObservation(ctx, envelope.Process, time.Now())
	g.Log().Info(ctx, "领取采集来源任务", fields)
	started := time.Now()
	workErr, leaseErr := awaitCollectSourceRuntimeClaim(ctx, queue, claim, lease, func(workCtx context.Context) error {
		if decodeErr != nil {
			return decodeErr
		}
		return handle(workCtx, envelope)
	})
	fields["event"] = "collect.source.settled"
	fields["executionDurationMs"] = time.Since(started).Milliseconds()
	if leaseErr != nil {
		fields["outcome"] = "lease_uncertain"
		fields["waitReason"] = "lease_lost"
		fields["error"] = leaseErr.Error()
		g.Log().Warning(ctx, "采集来源租约不确定，取消执行并保留过期恢复", fields)
		return
	}
	delay, reason := collectSourceRuntimeRetry(workErr)
	settleCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var err error
	if workErr != nil {
		err = queue.Nack(settleCtx, claim, delay)
		fields["outcome"] = "retry"
		fields["waitReason"] = reason
		fields["retryReason"] = collectRetryReason(workErr)
		fields["delayMs"] = delay.Milliseconds()
		fields["error"] = workErr.Error()
	} else {
		err = queue.Ack(settleCtx, claim)
		fields["outcome"] = "acknowledged"
		fields["waitReason"] = "none"
	}
	if err != nil {
		fields["outcome"] = "settlement_failed"
		fields["error"] = err.Error()
		g.Log().Warning(ctx, "采集来源任务确认失败，保留租约恢复", fields)
		return
	}
	if workErr != nil {
		g.Log().Info(ctx, "采集来源任务延期，释放来源执行槽", fields)
	} else {
		g.Log().Info(ctx, "采集来源任务确认完成", fields)
	}
}

func collectSourceRuntimeRetry(err error) (time.Duration, string) {
	if collectConfigurationFailure(err) {
		return collectConfigurationRetryDelay, "configuration_unavailable"
	}
	if err == nil {
		return 0, "none"
	}
	var retry *collectProcessRetryError
	if errors.As(err, &retry) {
		return max(collectProcessMinimumDelay, retry.delay), "job_dependency_wait"
	}
	var mediaRetry *collectMediaRetryError
	if errors.As(err, &mediaRetry) {
		return max(collectProcessMinimumDelay, mediaRetry.delay), "processing_error"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return collectProcessRedisRetryDelay, "execution_cancelled"
	}
	return collectProcessRedisRetryDelay, "processing_error"
}

func collectRetryReason(err error) string {
	if err == nil {
		return "none"
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "相同Telegram媒体正在由其他Worker处理"),
		strings.Contains(message, "获取采集事件媒体锁失败"):
		return "media_claim_wait"
	case strings.Contains(message, "媒体缓存中"),
		strings.Contains(message, "验证媒体缓存中"),
		strings.Contains(message, "媒体待就绪"):
		return "media_cache_pending"
	case strings.Contains(message, "媒体下载触发限流"),
		strings.Contains(message, "媒体下载临时失败"),
		strings.Contains(message, "媒体下载临时中断"):
		return "media_download_retry"
	case strings.Contains(message, "等待验证视频匹配"),
		strings.Contains(message, "验证资料未匹配到前序资料组"):
		return "verify_pair_wait"
	case strings.Contains(message, "配对区间已变化"),
		strings.Contains(message, "来源批次已合并"),
		strings.Contains(message, "采集批次有新消息"):
		return "material_window_wait"
	case strings.Contains(message, "等待来源Outbox后续调度"):
		return "outbox_wait"
	default:
		return "job_dependency_wait"
	}
}

// The notification remains leased until this bounded scoped dispatch pass is
// durable. A delayed Nack rechecks unfinished outbox work without a source loop.
func (s *sSysPublish) executeCollectSourceOutboxWake(ctx context.Context, p collectProcessQueuePayload) error {
	if err := s.dispatchDueCollectProcessOutbox(ctx, collectProcessDispatchBatchSize(ctx), publishAccountScope{TenantID: p.TenantId, AccountID: p.AccountId, SourceID: p.SourceId}); err != nil {
		return err
	}
	// Keep periodic source wakeups in explicit legacy mode. No new PG probe is
	// needed: the existing dispatcher remains authoritative for outbox status.
	return newCollectProcessRetryError(collectSourceManagerIntervalForRuntime(ctx), "等待来源Outbox后续调度")
}
func collectSourceManagerIntervalForRuntime(ctx context.Context) time.Duration {
	return time.Duration(collectSourceRuntimeConfig(ctx, "youbanPublish.collect.sourceManagerIntervalSeconds", 5, 1, 60)) * time.Second
}

func runCollectSourceRuntimeRecovery(ctx context.Context, queue collectSourceRuntimeQueue, lane sourcequeue.Lane) {
	limit := 64
	if lane == sourcequeue.History {
		limit = 16
	}
	for ctx.Err() == nil {
		recoverCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := queue.Recover(recoverCtx, lane, limit)
		cancel()
		if err != nil {
			g.Log().Warning(ctx, "恢复到期采集来源任务失败", g.Map{"event": "collect.source.recover.failed", "adapter": "sourcequeue", "queueLane": lane, "waitReason": "recovery_unavailable", "error": err.Error()})
		}
		if !waitCollectSourceRuntime(ctx, time.Second) {
			return
		}
	}
}

func collectSourceLegacyQueues(names []string) (map[string]int, error) {
	if len(names) > 16 {
		return nil, gerror.New("旧采集队列迁移一次最多指定16个队列")
	}
	queues := make(map[string]int, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		valid := name == tgQueueNameCollectProcess || name == tgQueueNameCollectProcessRealtime || name == tgQueueNameCollectProcessRealtimeV2 || name == tgQueueNameCollectProcessRecoveryV2 || strings.HasPrefix(name, tgQueueNameCollectProcessSourceRealtimePrefix) || strings.HasPrefix(name, tgQueueNameCollectProcessSourceRecoveryPrefix)
		if !valid || strings.ContainsAny(name, "*?[] \t\r\n") {
			return nil, gerror.New("旧采集队列迁移只能指定明确的采集处理队列")
		}
		queues[name] = 1
	}
	return queues, nil
}
