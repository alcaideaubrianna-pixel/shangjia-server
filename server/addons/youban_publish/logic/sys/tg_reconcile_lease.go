package sys

import (
	"context"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
	"hotgo/utility/ownedlease"

	"github.com/gogf/gf/v2/errors/gerror"
)

// withTelegramUnknownJobLease serializes webhook/recovery/worker reconciliation of
// one job. A processing checkpoint can be retried after a crash, but never acts
// as proof that the current caller owns execution.
func (s *sSysPublish) withTelegramUnknownJobLease(ctx context.Context, jobID int64, run func(context.Context) error) error {
	if jobID <= 0 {
		return gerror.New("TG对账任务ID不能为空")
	}
	ctx = telegramPublishTraceContext(ctx, jobID)
	client, err := cache.RedisClient()
	if err != nil {
		logTelegramQueueEvent(ctx, "reconcile_lease_acquire", g.Map{"stage": "connect"}, err)
		return gerror.Wrap(err, "获取TG对账租约连接失败")
	}
	lease := ownedlease.New(publishconsts.TelegramReconcileLeasePrefix+strconv.FormatInt(jobID, 10), "telegram_reconcile", telegramStreamExecutionTTL, client)
	return s.runTelegramReconcileLease(ctx, lease, run)
}

func (s *sSysPublish) runTelegramReconcileLease(ctx context.Context, lease *ownedlease.Lease, run func(context.Context) error) error {
	startedAt := time.Now()
	claimed, err := lease.TryAcquire(ctx)
	if err != nil {
		logTelegramQueueEvent(ctx, "reconcile_lease_acquire", g.Map{"stage": "acquire", "durationMs": time.Since(startedAt).Milliseconds()}, err)
		return gerror.Wrap(err, "领取TG对账租约失败")
	}
	if !claimed {
		logTelegramQueueEvent(ctx, "reconcile_lease_acquire", g.Map{"result": "busy", "reason": "owned_by_other_execution", "durationMs": time.Since(startedAt).Milliseconds()}, nil)
		return nil
	}
	logTelegramQueueEvent(ctx, "reconcile_lease_acquire", g.Map{"result": "acquired", "leaseMs": telegramStreamExecutionTTL.Milliseconds(), "durationMs": time.Since(startedAt).Milliseconds()}, nil)
	execution, stop := telegramStreamExecutionContext(ctx, lease)
	defer func() {
		stop()
		s.releaseTelegramChannelLease(ctx, lease)
	}()
	err = run(execution)
	logErr := err
	fields := g.Map{"stage": "reconcile", "durationMs": time.Since(startedAt).Milliseconds()}
	if execution.Err() != nil {
		fields["result"] = "cancelled"
		fields["reason"] = "execution_context_cancelled"
		fields["cancelled"] = true
		if logErr == nil {
			logErr = context.Cause(execution)
		}
	}
	logTelegramQueueEvent(ctx, "reconcile_execution_finished", fields, logErr)
	return err
}
