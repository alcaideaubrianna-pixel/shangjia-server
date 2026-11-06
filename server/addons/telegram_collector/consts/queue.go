package consts

import "fmt"

const (
	QueueCollectRealtime            = "tg.collect.realtime"
	QueueCollectUrgent              = "tg.collect.urgent"
	QueueCollectHistory             = "tg.collect.history"
	QueueMediaUrgent                = "tg.media.urgent"
	QueueMediaNormal                = "tg.media.normal"
	QueueMediaHistory               = "tg.media.history"
	QueueMediaAccount               = "tg.media.account"
	QueueMediaProcess               = "tg.media.process"
	QueueDeliveryRealtime           = "tg.delivery.realtime"
	QueueDeliveryHistory            = "tg.delivery.history"
	QueueDeliveryHistoryShardPrefix = "tg.delivery.history.account."
	QueueDeliveryReady              = "tg.delivery.ready"
	QueueDeliveryUrgent             = "tg.delivery.urgent"
	QueueDeliveryUrgentLive         = "tg.delivery.urgent.v2"
	QueuePublishUrgent              = "tg.publish.urgent"
	QueuePublishNormal              = "tg.publish.normal"
	QueuePublishRetry               = "tg.publish.retry"
	QueueRecovery                   = "tg.recovery"
	QueueDeadLetter                 = "tg.deadletter"
)

const DeliveryHistoryShardCount = 64

func DeliveryHistoryShardQueue(accountID int64) string {
	if accountID < 0 {
		accountID = -accountID
	}
	return fmt.Sprintf("%s%02d", QueueDeliveryHistoryShardPrefix, accountID%DeliveryHistoryShardCount)
}
