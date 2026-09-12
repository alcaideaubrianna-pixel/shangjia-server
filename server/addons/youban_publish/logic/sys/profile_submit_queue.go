package sys

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type profileSubmitPayload struct {
	TenantId  int64                `json:"tenantId"`
	AccountId int64                `json:"accountId"`
	Input     sysin.ProfileSaveInp `json:"input"`
}

func (s *sSysPublish) enqueueProfileSubmit(ctx context.Context, in *sysin.ProfileSaveInp, tenantId, accountId int64) (*sysin.ProfileSaveModel, error) {
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("资料草稿不存在，请重新上传")
	}
	payload := profileSubmitPayload{TenantId: tenantId, AccountId: accountId, Input: *in}
	payload.Input.AsyncSubmit = false
	payload.Input.DraftOnly = false
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, gerror.Wrap(err, "编码资料保存任务失败")
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return nil, err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeProfileSubmit, body),
		asynq.Queue(tgQueueNameProfileMaintenance), asynq.MaxRetry(8), asynq.Timeout(5*time.Minute))
	if err != nil {
		return nil, gerror.Wrap(err, "提交资料保存任务失败")
	}
	g.Log().Info(ctx, "资料保存任务已提交", g.Map{"profileId": in.Id, "tenantId": tenantId, "accountId": accountId, "publish": in.PublishAfterSave})
	return &sysin.ProfileSaveModel{Id: in.Id, Uuid: normalizeProfileUUID(in.Uuid)}, nil
}

func (s *sSysPublish) handleProfileSubmitTask(ctx context.Context, task *asynq.Task) error {
	var payload profileSubmitPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return gerror.Wrap(err, "解析资料保存任务失败")
	}
	if payload.TenantId <= 0 || payload.AccountId <= 0 || payload.Input.Id <= 0 {
		return gerror.New("资料保存任务参数不完整")
	}
	publishAfterSave := payload.Input.PublishAfterSave
	payload.Input.AsyncSubmit = false
	payload.Input.DraftOnly = false
	payload.Input.PublishAfterSave = false
	saved, err := s.saveProfile(ctx, &payload.Input, payload.TenantId, payload.AccountId)
	if err != nil {
		return gerror.Wrap(err, "后台保存资料失败")
	}
	if publishAfterSave {
		if err = s.submitProfilePublish(ctx, saved.Id, payload.TenantId, payload.AccountId, 0, "", nil, false); err != nil {
			return gerror.Wrap(err, "后台提交资料上架失败")
		}
	}
	g.Log().Info(ctx, "资料保存任务完成", g.Map{"profileId": saved.Id, "tenantId": payload.TenantId, "accountId": payload.AccountId, "publish": publishAfterSave})
	return nil
}
