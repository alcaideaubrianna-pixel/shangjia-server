package queues

import (
	"context"
	"encoding/json"

	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/addons/lazysheep_tggo/service"
	"hotgo/internal/library/queue"
)

func init() {
	queue.RegisterConsumer(WebhookUpdate)
}

var WebhookUpdate = &qWebhookUpdate{}

type qWebhookUpdate struct{}

func (q *qWebhookUpdate) GetTopic() string { return sysin.WebhookUpdateTopic }

func (q *qWebhookUpdate) Handle(ctx context.Context, mqMsg queue.MqMsg) error {
	var task sysin.WebhookUpdateTask
	if err := json.Unmarshal(mqMsg.Body, &task); err != nil {
		return err
	}
	return service.SysLazysheepTggo().ProcessWebhook(ctx, task.BotKey, task.Payload)
}
