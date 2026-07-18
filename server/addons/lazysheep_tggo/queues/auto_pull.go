package queues

import (
	"context"
	"encoding/json"

	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/addons/lazysheep_tggo/service"
	"hotgo/internal/library/queue"
)

func init() {
	queue.RegisterConsumer(AutoPull)
}

var AutoPull = &qAutoPull{}

type qAutoPull struct{}

func (q *qAutoPull) GetTopic() string { return "lazysheep_tggo:auto_pull" }

func (q *qAutoPull) Handle(ctx context.Context, mqMsg queue.MqMsg) error {
	var task sysin.AutoPullTask
	if err := json.Unmarshal(mqMsg.Body, &task); err != nil {
		return err
	}
	if err := service.SysLazysheepTggo().HandleAutoPullTask(ctx, &task); err != nil {
		g.Log().Warningf(ctx, "自动拉取执行失败，等待下一轮调度 bot:%s binding:%s err:%+v", task.BotKey, task.BindingKey, err)
		return nil
	}
	return nil
}
