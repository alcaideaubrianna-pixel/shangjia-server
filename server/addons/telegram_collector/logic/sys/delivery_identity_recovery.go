package sys

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

// recoverConflictingDelivery hands the current raw update to its own durable
// delivery before retiring the stale reference. Crashes at any point are safe:
// save is idempotent and queue consumers claim the replacement by lease.
func (s *sCollector) recoverConflictingDelivery(ctx context.Context, old *entity.TgCollectorDelivery, event *entity.TgCollectorEvent) error {
	replacement, err := collectorDeliveryFromEvent(event, []byte(event.RawUpdate.String()))
	if err != nil {
		return err
	}
	id, err := s.saveDelivery(ctx, &replacement)
	if err != nil {
		return err
	}
	if id == old.Id {
		return gerror.New("恢复交付仍指向旧任务，等待重试")
	}
	// Recovery must really enqueue; a leftover Unique marker is not an ACK.
	if err = s.enqueueDeliveryTaskWithOptions(ctx, id, deliveryPriority(&replacement), false); err != nil {
		return gerror.Wrap(err, "恢复交付已保存，等待重新入队")
	}
	cols := dao.TgCollectorDelivery.Columns()
	result, err := dao.TgCollectorDelivery.Ctx(ctx).WherePri(old.Id).
		Where(cols.TenantId, old.TenantId).Where(cols.EventId, old.EventId).
		Where(cols.Status, sysin.DeliveryStatusProcessing).Where(cols.LeaseOwner, old.LeaseOwner).
		Data(g.Map{
			cols.Status:     sysin.DeliveryStatusQuarantined,
			cols.LeaseOwner: "", cols.LeaseUntil: nil, cols.NextRunAt: nil,
			cols.ErrorMessage: fmt.Sprintf("原始事件身份冲突，已交接恢复Delivery:%d", id),
			cols.UpdatedAt:    gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "恢复交付已入队，收口旧任务失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认冲突交付收口失败")
	}
	logCollectorDeliveryStage(ctx, "delivery.identity_recovered", old.Id, event.Id, collectorQueueName(ctx), nil,
		g.Map{"replacementDeliveryId": id, "oldDeliverySettled": affected == 1,
			"tenantId": event.TenantId, "sourceId": event.SourceId, "sourceMessageId": event.MessageId})
	return nil
}
