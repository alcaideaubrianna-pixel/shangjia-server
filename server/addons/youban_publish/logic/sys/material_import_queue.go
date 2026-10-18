package sys

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/hibiken/asynq"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const tgTaskTypeMaterialImport = "youban_publish:material_import"

type materialImportQueuePayload struct {
	TaskId int64 `json:"taskId"`
}

func (s *sSysPublish) enqueueMaterialImportTask(ctx context.Context, taskId int64, delay time.Duration) error {
	if taskId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(materialImportQueuePayload{TaskId: taskId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeMaterialImport, body)
	queueName, err := s.materialImportQueueName(ctx, taskId)
	if err != nil {
		return err
	}
	options := []asynq.Option{
		asynq.Queue(queueName),
		asynq.MaxRetry(0),
		asynq.Timeout(6 * time.Hour),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) materialImportQueueName(ctx context.Context, taskId int64) (string, error) {
	cols := pdao.YoubanPublishMaterialImportTask.Columns()
	value, err := pdao.YoubanPublishMaterialImportTask.Ctx(ctx).
		Where(cols.Id, taskId).
		Fields(cols.Stage).
		Value()
	if err != nil {
		return "", gerror.Wrap(err, "读取资料导入任务阶段失败")
	}
	return materialImportQueueNameByStage(value.String()), nil
}

func materialImportQueueNameByStage(stage string) string {
	if strings.TrimSpace(stage) == sysin.MaterialImportStageMedia {
		return tgQueueNameMaterialImportMedia
	}
	return tgQueueNameMaterialImportPull
}

func decodeMaterialImportQueuePayload(task *asynq.Task) (materialImportQueuePayload, error) {
	var payload materialImportQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, err
	}
	if payload.TaskId <= 0 {
		return payload, gerror.New("资料导入队列任务缺少taskId")
	}
	return payload, nil
}
