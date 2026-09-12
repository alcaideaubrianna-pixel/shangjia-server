package sys

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	"hotgo/internal/service"
)

type profileMaintenancePayload struct {
	ProfileId       int64   `json:"profileId"`
	TenantId        int64   `json:"tenantId"`
	AccountId       int64   `json:"accountId"`
	RemovedMediaIds []int64 `json:"removedMediaIds,omitempty"`
	MediaChanged    bool    `json:"mediaChanged,omitempty"`
}

func (s *sSysPublish) enqueueProfileMaintenance(ctx context.Context, payload profileMaintenancePayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return gerror.Wrap(err, "编码资料维护任务失败")
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeProfileMaintenance, body),
		asynq.Queue(tgQueueNameProfileMaintenance), asynq.MaxRetry(8), asynq.Timeout(2*time.Minute))
	return err
}

func (s *sSysPublish) handleProfileMaintenanceTask(ctx context.Context, task *asynq.Task) error {
	var payload profileMaintenancePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return gerror.Wrap(err, "解析资料维护任务失败")
	}
	if payload.ProfileId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
		return gerror.New("资料维护任务参数不完整")
	}
	count, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("profile_id", payload.ProfileId).Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).WhereNull("deleted_at").Count()
	if err != nil || count == 0 {
		return err
	}
	for _, mediaId := range uniqueIds(payload.RemovedMediaIds) {
		if err = s.deleteMediaPHashBucketByMediaId(ctx, mediaId); err != nil {
			return err
		}
	}
	if payload.MediaChanged {
		if err = s.syncMediaPHashBucketsByProfileId(ctx, payload.ProfileId); err != nil {
			return err
		}
	}
	channelIds, err := s.profileChannelIdsOrDefaults(ctx, payload.TenantId, payload.AccountId, payload.ProfileId)
	if err != nil {
		return err
	}
	if err = s.supersedeProfilePendingTelegramJobsOutsideChannels(ctx, payload.ProfileId, payload.TenantId, payload.AccountId, channelIds); err != nil {
		return err
	}
	if err = s.syncProfileNoteIndex(ctx, payload.ProfileId); err != nil {
		return err
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}
