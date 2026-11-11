package sys

import (
	"context"
	"database/sql"
	"errors"

	"hotgo/addons/youban_publish/library/streamgray"
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

func (d publishStreamGrayJobDispatcher) Dispatch(ctx context.Context, jobID int64) error {
	if d.publish == nil {
		return streamgray.ErrInvalidRuntime
	}
	err := d.publish.enqueueTelegramJobDirectWithUnique(ctx, jobID, 0, false)
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
