package sys

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/library/accountschedule"
	"hotgo/internal/library/cache"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

func accountScheduleRedis() (*accountschedule.Queue, error) {
	client, err := cache.RedisClient()
	if err != nil {
		return nil, err
	}
	seconds := g.Cfg().MustGet(context.Background(), "youbanPublish.queue.accountScheduleIdleIntervalSeconds", 300).Int()
	if seconds <= 0 {
		seconds = 300
	}
	return accountschedule.NewWithIdleInterval(client, publishconsts.AccountScheduleRedisPrefix, time.Duration(seconds)*time.Second), nil
}

// markAccountSchedule 合并扫描；实际业务变更通过 changed 保留执行期间的新唤醒。
func (s *sSysPublish) markAccountSchedule(ctx context.Context, scope publishAccountScope, changed bool) error {
	q, err := accountScheduleRedis()
	if err != nil {
		return err
	}
	wake := accountschedule.Ensure
	if changed {
		wake = accountschedule.Changed
	}
	return q.Mark(ctx, accountschedule.Scope{TenantID: scope.TenantID, AccountID: scope.AccountID}, wake)
}

// handleAccountScheduleTask 兼容滚动部署时旧队列：仅桥接标记并ACK，不运行旧调度。
// 新消费者仍与旧版本共用 AccountScheduleLeasePrefix，防止滚动期间同账户并行执行。
func (s *sSysPublish) handleAccountScheduleTask(ctx context.Context, task *asynq.Task) error {
	var scope publishAccountScope
	if err := json.Unmarshal(task.Payload(), &scope); err != nil {
		return asynq.SkipRetry
	}
	if scope.TenantID <= 0 || scope.AccountID <= 0 {
		return asynq.SkipRetry
	}
	return s.markAccountSchedule(ctx, scope, false)
}

// runAccountScheduleRedis 由Scheduler或Background角色启动；实例间由Redis token及现有ownedlease串行化。
// 固定worker数，每账户25秒预算，45秒claim租约，不受旧Asynq积压影响。
func (s *sSysPublish) runAccountScheduleRedis(ctx context.Context) {
	n := g.Cfg().MustGet(ctx, "youbanPublish.queue.accountScheduleConcurrency", 4).Int()
	if n < 1 {
		n = 1
	}
	if n > 32 {
		n = 32
	}
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func() { defer func() { done <- struct{}{} }(); s.accountScheduleRedisWorker(ctx) }()
	}
	for i := 0; i < n; i++ {
		<-done
	}
}
func (s *sSysPublish) accountScheduleRedisWorker(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		q, err := accountScheduleRedis()
		var claim *accountschedule.Claim
		if err == nil {
			claim, err = q.Claim(ctx, 45*time.Second)
		}
		if err != nil {
			g.Log().Warningf(ctx, "账户Redis调度领取失败 err:%+v", err)
			timer.Reset(time.Second)
			continue
		}
		if claim == nil {
			timer.Reset(250 * time.Millisecond)
			continue
		}
		s.executeAccountScheduleClaim(ctx, q, *claim)
		timer.Reset(time.Millisecond)
	}
}
func (s *sSysPublish) executeAccountScheduleClaim(ctx context.Context, q *accountschedule.Queue, claim accountschedule.Claim) {
	outcome := accountschedule.Failure
	defer func() {
		if recovered := recover(); recovered != nil {
			g.Log().Warningf(ctx, "账户Redis调度异常 accountId:%d panic:%v", claim.Scope.AccountID, recovered)
		}
		ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		ok, err := q.Ack(ackCtx, claim, outcome)
		g.Log().Info(ctx, "账户Redis调度收口", g.Map{"tenantId": claim.Scope.TenantID, "accountId": claim.Scope.AccountID, "generation": claim.Generation, "outcome": outcome, "acked": ok, "error": err})
	}()
	bounded, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	// Only absent/deleted accounts are retired. Disabled accounts can still need cleanup.
	accountID, err := g.DB().Model(publishAccountTable).Ctx(bounded).Fields("id").
		Where("tenant_id", claim.Scope.TenantID).Where("id", claim.Scope.AccountID).WhereNull("deleted_at").Value()
	if err != nil {
		g.Log().Warningf(ctx, "核验调度账户失败 accountId:%d err:%+v", claim.Scope.AccountID, err)
		return
	}
	if accountID.IsNil() || accountID.Int64() <= 0 {
		outcome = accountschedule.Retired
		return
	}
	err = s.executeAccountSchedule(bounded, publishAccountScope{TenantID: claim.Scope.TenantID, AccountID: claim.Scope.AccountID}, claim.Turn-1)
	outcome = accountScheduleResult(err, bounded.Err())
	if outcome == accountschedule.Stalled {
		g.Log().Warning(ctx, "账户调度预算耗尽，保留任务并退避重试", g.Map{"tenantId": claim.Scope.TenantID, "accountId": claim.Scope.AccountID, "turn": claim.Turn})
	}
	if outcome == accountschedule.Failure {
		g.Log().Warningf(ctx, "账户Redis调度执行失败 tenantId:%d accountId:%d err:%v context:%v", claim.Scope.TenantID, claim.Scope.AccountID, err, bounded.Err())
	}
}

// scheduleAccountCycleWake 在持久计划索引成功后登记账户最早到期时间，不读取全局任务表。
// 索引生产者应调用此方法；多个计划只保留一个最早唤醒，到期后消费账户自己的索引。
func scheduleAccountCycleWake(ctx context.Context, scope publishAccountScope, at time.Time) error {
	q, err := accountScheduleRedis()
	if err != nil {
		return err
	}
	return q.Schedule(ctx, accountschedule.Scope{TenantID: scope.TenantID, AccountID: scope.AccountID}, at)
}

// accountScheduleRealError inspects every joined leaf before interpreting continuation signals.
// A wrapped Join(More, databaseError) must not clear failure counters via the More branch.
func accountScheduleRealError(err, budgetErr error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, child := range joined.Unwrap() {
			if accountScheduleRealError(child, budgetErr) {
				return true
			}
		}
		return false
	}
	if child := errors.Unwrap(err); child != nil {
		return accountScheduleRealError(child, budgetErr)
	}
	if errors.Is(err, errAccountScheduleMore) || errors.Is(err, errAccountScheduleBusy) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) && errors.Is(budgetErr, context.DeadlineExceeded) {
		return false
	}
	return true
}
func accountScheduleResult(err, budgetErr error) accountschedule.Outcome {
	if accountScheduleRealError(err, budgetErr) {
		return accountschedule.Failure
	}
	if errors.Is(budgetErr, context.DeadlineExceeded) {
		return accountschedule.Stalled
	}
	if errors.Is(err, errAccountScheduleMore) {
		return accountschedule.More
	}
	if errors.Is(err, errAccountScheduleBusy) {
		return accountschedule.Deferred
	}
	if err == nil && budgetErr == nil {
		return accountschedule.Success
	}
	return accountschedule.Failure
}
