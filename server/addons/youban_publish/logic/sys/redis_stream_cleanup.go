package sys

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/utility/ownedlease"
)

// withTelegramCleanupExecution 统一新引擎的独立频道删除槽位，网络调用期间不持有数据库事务或发送租约。
// 旧引擎沿用原锁；新引擎共用现有删除槽位键，避免灰度期间删除并发超出上限。
func (s *sSysPublish) withTelegramCleanupExecution(ctx context.Context, job telegramJobRecord, fn func(context.Context) error) error {
	if s.accountPushEngine(ctx, job.TenantId, job.AccountId) != pushEngineUnifiedStream {
		return s.withTelegramChannelLock(ctx, job.TargetChatId, func() error { return fn(ctx) })
	}
	for slot := 0; slot < telegramCleanupPerChannelConcurrency; slot++ {
		lease := ownedlease.New(fmt.Sprintf("%s:cleanup:%d", telegramChannelLockKey(job.TargetChatId), slot), "telegram_cleanup_stream_v2", telegramStreamExecutionTTL)
		acquired, err := lease.TryAcquire(ctx)
		if err != nil {
			return gerror.Wrap(err, "获取频道删除执行槽位失败")
		}
		if !acquired {
			continue
		}
		bounded, stop := telegramStreamExecutionContext(ctx, lease)
		defer func() { stop(); s.releaseTelegramChannelLease(ctx, lease) }()
		return fn(bounded)
	}
	return &tgRetryAfterError{after: telegramStreamExecutionRenewInterval, err: gerror.New("该频道删除槽位繁忙，等待重试")}
}
