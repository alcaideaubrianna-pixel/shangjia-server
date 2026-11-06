package streamgray

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

// JobDispatcher 是现有 Asynq 入队能力在 Stream 层的最小适配接口。
// 实现必须幂等，并复用频道租约、顺序检查和任务状态校验。
type JobDispatcher interface {
	Dispatch(context.Context, int64) error
}

// DispatchReadyEvent 将 Stream 事件交给现有任务调度器。
// 重复事件不会在此层去重，幂等性由 JobDispatcher（通常是数据库状态条件更新 + Asynq 唯一入队）保证。
func DispatchReadyEvent(ctx context.Context, dispatcher JobDispatcher, event ReadyEvent) error {
	if dispatcher == nil {
		return gerror.New("Redis Stream job dispatcher 未初始化")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	return dispatcher.Dispatch(ctx, event.JobID)
}
