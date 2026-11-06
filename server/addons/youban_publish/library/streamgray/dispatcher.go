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

// AccountGate 用于在消费时重新读取账户灰度状态。
// 返回 false 表示账户已回滚或事件不再属于当前灰度范围。
type AccountGate interface {
	Enabled(context.Context, int64, int64) (bool, error)
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

// DispatchReadyEventWithGate 在任务入队前校验租户账户灰度状态。
// 该函数不负责 ACK；调用方应在返回 nil 后 ACK，返回错误则保留 pending。
func DispatchReadyEventWithGate(ctx context.Context, gate AccountGate, dispatcher JobDispatcher, event ReadyEvent) error {
	if gate == nil {
		return gerror.New("Redis Stream account gate 未初始化")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	enabled, err := gate.Enabled(ctx, event.TenantID, event.AccountID)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	return DispatchReadyEvent(ctx, dispatcher, event)
}
