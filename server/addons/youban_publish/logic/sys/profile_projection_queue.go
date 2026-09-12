package sys

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/internal/dao"
	"hotgo/internal/service"
)

type profileProjectionQueuePayload struct {
	ProfileId       int64                `json:"profileId"`
	TenantId        int64                `json:"tenantId"`
	AccountId       int64                `json:"accountId"`
	OldFingerprints []profileFingerprint `json:"oldFingerprints,omitempty"`
	RemovedMediaIds []int64              `json:"removedMediaIds,omitempty"`
	MediaChanged    bool                 `json:"mediaChanged,omitempty"`
}

func (s *sSysPublish) enqueueProfileProjection(ctx context.Context, payload profileProjectionQueuePayload) error {
	if payload.ProfileId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
		return gerror.New("资料投影任务参数不完整")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return gerror.Wrap(err, "编码资料投影任务失败")
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeProfileProjection, body),
		asynq.Queue(tgQueueNameProfileProjection), asynq.MaxRetry(8), asynq.Timeout(2*time.Minute))
	return err
}

func decodeProfileProjectionQueuePayload(task *asynq.Task) (profileProjectionQueuePayload, error) {
	var payload profileProjectionQueuePayload
	if task == nil {
		return payload, gerror.New("资料投影任务不能为空")
	}
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, gerror.Wrap(err, "解析资料投影任务失败")
	}
	if payload.ProfileId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
		return payload, gerror.New("资料投影任务参数不完整")
	}
	return payload, nil
}

func (s *sSysPublish) handleProfileProjectionTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeProfileProjectionQueuePayload(task)
	if err != nil {
		return err
	}
	stateCount, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Where("profile_id", payload.ProfileId).Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "检查资料投影源状态失败")
	}
	if stateCount == 0 {
		g.Log().Infof(ctx, "跳过已删除的资料投影任务 profileId:%d tenantId:%d accountId:%d", payload.ProfileId, payload.TenantId, payload.AccountId)
		return nil
	}

	clearProfileFingerprintCache(ctx, payload.TenantId, payload.AccountId, payload.OldFingerprints)
	currentFingerprints, err := profileFingerprintsByProfile(ctx, payload.ProfileId, payload.TenantId, payload.AccountId)
	if err != nil {
		return err
	}
	warmProfileFingerprintCache(ctx, payload.TenantId, payload.AccountId, payload.ProfileId, currentFingerprints)

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
	// Note index is the completion marker used by recovery. Keep it after all
	// other durable projection work so a partial run remains discoverable.
	if err = s.syncProfileNoteIndex(ctx, payload.ProfileId); err != nil {
		return err
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	g.Log().Infof(ctx, "资料异步投影完成 profileId:%d tenantId:%d accountId:%d", payload.ProfileId, payload.TenantId, payload.AccountId)
	return nil
}

func (s *sSysPublish) runProfileProjectionRecovery(ctx context.Context) {
	s.recoverProfileProjectionTasks(ctx, 200)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverProfileProjectionTasks(ctx, 200)
		}
	}
}

func (s *sSysPublish) recoverProfileProjectionTasks(ctx context.Context, limit int) {
	if limit <= 0 {
		return
	}
	var rows []struct {
		ProfileId int64 `json:"profile_id"`
		TenantId  int64 `json:"tenant_id"`
		AccountId int64 `json:"account_id"`
	}
	profileColumns := dao.ContentProfile.Columns()
	indexColumns := pdao.YoubanPublishNoteIndex.Columns()
	err := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		LeftJoin(pdao.YoubanPublishNoteIndex.Table()+" i", "i."+indexColumns.ProfileId+"=p."+profileColumns.Id+" AND i."+indexColumns.TenantId+"=ps.tenant_id AND i."+indexColumns.AccountId+"=ps.account_id AND i."+indexColumns.DeletedAt+" IS NULL").
		Fields("p." + profileColumns.Id + " AS profile_id,ps.tenant_id,ps.account_id").
		WhereNull("p." + profileColumns.DeletedAt).
		Where("(i." + indexColumns.Id + " IS NULL OR i." + indexColumns.SourceUpdatedAt + " IS NULL OR i." + indexColumns.SourceUpdatedAt + " < p." + profileColumns.UpdatedAt + ")").
		OrderAsc("p." + profileColumns.UpdatedAt).Limit(limit).Scan(&rows)
	if err != nil {
		g.Log().Warningf(ctx, "扫描待恢复资料投影失败 err:%+v", err)
		return
	}
	for _, row := range rows {
		if err = s.enqueueProfileProjection(ctx, profileProjectionQueuePayload{
			ProfileId: row.ProfileId, TenantId: row.TenantId, AccountId: row.AccountId, MediaChanged: true,
		}); err != nil {
			g.Log().Warningf(ctx, "恢复资料投影任务入队失败 profileId:%d tenantId:%d accountId:%d err:%+v", row.ProfileId, row.TenantId, row.AccountId, err)
		}
	}
}

func profileFingerprintsByProfile(ctx context.Context, profileId, tenantId, accountId int64) ([]profileFingerprint, error) {
	rows, err := g.DB().Model(publishProfileFingerprintTable).Safe().Ctx(ctx).
		Fields("channel_id,layer,signature,item_total,signature_count").
		Where("profile_id", profileId).Where("tenant_id", tenantId).Where("account_id", accountId).
		Where("owner_marker", "owner").WhereNot("layer", "_indexed").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料指纹投影失败")
	}
	items := make([]profileFingerprint, 0, len(rows))
	for _, row := range rows {
		items = append(items, profileFingerprint{
			ChannelID: row["channel_id"].Int64(), Layer: row["layer"].String(), Signature: row["signature"].String(),
			ItemTotal: row["item_total"].Int(), SignatureCount: row["signature_count"].Int(),
		})
	}
	return items, nil
}
