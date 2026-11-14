package sys

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/library/streamgray"
	"hotgo/internal/library/cache"
)

// publishStreamGrayAccountGate 将现有账户推送引擎配置适配为 Stream 门禁。
type publishStreamGrayAccountGate struct{ publish *sSysPublish }

func (g publishStreamGrayAccountGate) Enabled(ctx context.Context, tenantID, accountID int64) (bool, error) {
	if g.publish == nil {
		return false, nil
	}
	return g.publish.accountPushEngine(ctx, tenantID, accountID) == pushEngineUnifiedStream, nil
}

// publishStreamGrayJobDispatcher 将 Stream 事件交回现有幂等 Asynq 调度入口。
type publishStreamGrayJobDispatcher struct{ publish *sSysPublish }

const publishStreamDispatchDebounce = 300 * time.Millisecond

func (d publishStreamGrayJobDispatcher) Dispatch(ctx context.Context, jobID int64) error {
	if d.publish == nil {
		return streamgray.ErrInvalidRuntime
	}
	// Outbox 恢复、Stream 重投和频道完成回调可能产生同一个 job 的多个事件。
	// 短窗口去重只减少重复查库/抢锁，事件可靠性仍由 Outbox 到期恢复保证。
	if cache.Initialized() {
		ok, cacheErr := cache.Instance().SetIfNotExist(ctx,
			fmt.Sprintf("youban_publish:tg:stream-dispatch:%d", jobID), 1,
			publishStreamDispatchDebounce)
		if cacheErr != nil {
			// 缓存故障不能阻断持久化任务调度，继续走正常幂等路径。
			g.Log().Warningf(ctx, "TG Stream任务去重缓存失败 jobId:%d err:%+v", jobID, cacheErr)
		} else if !ok {
			return nil
		}
	}
	err := d.publish.EnqueueTelegramJob(ctx, jobID, 0, false)
	// 已删除任务的旧通知可以确认；数据库连接故障仍返回错误，不能丢通知。
	if errors.Is(err, sql.ErrNoRows) || (err != nil && err.Error() == "TG任务不存在") {
		return nil
	}
	return err
}

// newPublishStreamGrayBridge 创建灰度 Consumer 所需的业务适配器。
// 该函数不改变旧调度路径，只有运行时显式注入后才会生效。
func (s *sSysPublish) newPublishStreamGrayBridge() (streamgray.AccountGate, streamgray.JobDispatcher) {
	return publishStreamGrayAccountGate{publish: s}, publishStreamGrayJobDispatcher{publish: s}
}
