package queues

import (
	"context"
	"encoding/json"

	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/addons/lazysheep_tggo/service"
	"hotgo/internal/library/queue"
)

func init() {
	queue.RegisterConsumer(PushNote)
}

var PushNote = &qPushNote{}

type qPushNote struct{}

func (q *qPushNote) GetTopic() string { return "lazysheep_tggo:push_note" }

func (q *qPushNote) Handle(ctx context.Context, mqMsg queue.MqMsg) error {
	var task sysin.PushNoteTask
	if err := json.Unmarshal(mqMsg.Body, &task); err != nil {
		return err
	}
	service.SysLazysheepTggo().DispatchPushNoteTask(ctx, &task)
	return nil
}
