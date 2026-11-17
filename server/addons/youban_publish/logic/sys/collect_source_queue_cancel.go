package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/utility/sourcequeue"
)

// Call only after durable source/event cancellation. This invalidates source
// queue tokens without inspecting another source or enumerating pending tasks.
func cancelCollectSourceExecutionQueues(ctx context.Context, tenantID, sourceID int64) error {
	if tenantID <= 0 || sourceID <= 0 {
		return gerror.New("取消采集队列必须指定租户和来源")
	}
	scope := sourcequeue.Scope{TenantID: tenantID, SourceID: sourceID}
	for _, namespace := range []string{sourcequeue.CollectorEventNamespace, sourcequeue.CollectorDeliveryNamespace, sourcequeue.CollectProcessNamespace} {
		queue, err := sourcequeue.New(namespace)
		if err != nil {
			return gerror.Wrap(err, "初始化待取消的来源队列失败")
		}
		if err := queue.CancelScope(ctx, scope); err != nil {
			return gerror.Wrapf(err, "取消来源执行队列失败 namespace:%s", namespace)
		}
	}
	return nil
}
