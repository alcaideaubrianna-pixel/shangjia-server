package streamgray

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

// CommitFunc 表示已经完成数据库提交的回调。
// 事务内部不得调用 Producer；调用方应在事务成功返回后再调用 PublishReadyEvent。
type CommitFunc func(context.Context) error

// PublishAfterCommit 统一表达“先提交持久化状态，再发布 Stream 通知”的顺序。
// commit 失败时不会写 Redis；通知失败只返回错误，由 Outbox 恢复扫描再次投递。
func PublishAfterCommit(ctx context.Context, commit CommitFunc, producer Producer, stream string, event ReadyEvent) (string, error) {
	if commit == nil {
		return "", gerror.New("数据库提交回调未初始化")
	}
	if err := commit(ctx); err != nil {
		return "", err
	}
	return PublishReadyEvent(ctx, producer, stream, event)
}
