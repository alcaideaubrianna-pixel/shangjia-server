package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const collectSourceDownMessage = "采集源一键下架"

const collectSourceDownDeleteBatchSize = 200

func (s *sSysPublish) CollectSourceProfileIds(ctx context.Context, in *sysin.CollectSourceProfileIdsInp) (*sysin.CollectSourceProfileIdsModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("采集源ID不能为空")
	}
	source, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", in.Id).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集源失败")
	}
	if source.IsEmpty() {
		return nil, gerror.New("采集源不存在")
	}
	ids, err := s.collectSourceProfileIds(ctx, in.Id, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []int64{}
	}
	return &sysin.CollectSourceProfileIdsModel{Ids: ids}, nil
}

func (s *sSysPublish) CollectSourceDown(ctx context.Context, in *sysin.CollectSourceDownInp) (*sysin.CollectSourceDownModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("采集源ID不能为空")
	}
	source, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", in.Id).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集源失败")
	}
	if source.IsEmpty() {
		return nil, gerror.New("采集源不存在")
	}
	now := gtime.Now()
	_, err = pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", in.Id).
		Data(g.Map{"collect_enabled": 0, "updated_by": account.Id, "updated_at": now}).
		Update()
	if err != nil {
		return nil, gerror.Wrap(err, "停止采集源失败")
	}
	profileIds, err := s.collectSourceProfileIds(ctx, in.Id, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	res, err := s.collectSourceDownPreview(ctx, in.Id, account.TenantId, account.Id, profileIds)
	if err != nil {
		return nil, err
	}
	if len(profileIds) == 0 {
		return res, nil
	}
	if err = s.supersedeCollectSourcePendingJobs(ctx, profileIds, account.TenantId); err != nil {
		return nil, err
	}
	if err = s.enqueueCollectSourceDown(ctx, collectSourceDownQueuePayload{
		AccountId:      account.Id,
		DeleteProfiles: in.DeleteProfiles,
		SourceId:       in.Id,
		TenantId:       account.TenantId,
	}, 0); err != nil {
		return nil, gerror.Wrap(err, "投递采集源下架任务失败")
	}
	res.Queued = 1
	return res, nil
}

// BotCollectSourceDown is the explicit-scope adapter used by Telegram Bot.
// Queueing and deletion remain implemented by the existing source-down flow.
func (s *sSysPublish) BotCollectSourceDown(ctx context.Context, sourceId, tenantId, accountId int64, deleteProfiles bool) (*sysin.CollectSourceDownModel, error) {
	if sourceId <= 0 || tenantId <= 0 || accountId <= 0 {
		return nil, gerror.New("采集源参数不完整")
	}
	if _, err := pdao.YoubanPublishCollectSource.Ctx(ctx).Where("id", sourceId).Where("tenant_id", tenantId).Where("account_id", accountId).WhereNull("deleted_at").One(); err != nil {
		return nil, gerror.Wrap(err, "读取采集源失败")
	}
	return s.ExecuteCollectSourceDown(ctx, sourceId, tenantId, accountId, deleteProfiles)
}

func (s *sSysPublish) BotCollectSourceDownAsync(ctx context.Context, sourceId, tenantId, accountId int64, deleteProfiles bool) (*sysin.CollectSourceDownModel, error) {
	if sourceId <= 0 || tenantId <= 0 || accountId <= 0 {
		return nil, gerror.New("采集源参数不完整")
	}
	return s.CollectSourceDown(ctx, &sysin.CollectSourceDownInp{Id: sourceId, DeleteProfiles: deleteProfiles})
}

func (s *sSysPublish) ExecuteCollectSourceDown(ctx context.Context, sourceId int64, tenantId int64, accountId int64, deleteProfiles bool) (*sysin.CollectSourceDownModel, error) {
	profileIds, err := s.collectSourceProfileIds(ctx, sourceId, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	res, err := s.collectSourceDownPreview(ctx, sourceId, tenantId, accountId, profileIds)
	if err != nil {
		return nil, err
	}
	if len(profileIds) == 0 {
		return res, nil
	}
	jobs, err := s.collectSourceDownJobs(ctx, profileIds, tenantId)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		if job.Status == "sent" || res.MessageCount > 0 {
			if err = s.deleteTelegramMessageSet(ctx, job.telegramJobRecord(), collectSourceDownMessage); err != nil {
				// A failed Telegram cleanup must not prevent local profiles from
				// being taken offline. The message may already be deleted or the
				// bot may lack permission; record it and continue the state update.
				g.Log().Warningf(ctx, "采集源下架清理TG消息失败，继续下架资料 jobId:%d err:%v", job.Id, err)
			}
		}
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			g.Log().Warningf(ctx, "采集源下架废弃TG任务失败，继续下架资料 jobId:%d err:%v", job.Id, err)
			continue
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "down", "deleted", "采集源一键下架，目标频道消息已删除或任务已废弃")
	}
	if err = s.finishCollectSourceDownProfiles(ctx, profileIds, tenantId); err != nil {
		return nil, err
	}
	if err = s.finishCollectSourceDownDispatch(ctx, sourceId, profileIds, tenantId, accountId); err != nil {
		return nil, err
	}
	if deleteProfiles {
		for start := 0; start < len(profileIds); start += collectSourceDownDeleteBatchSize {
			end := start + collectSourceDownDeleteBatchSize
			if end > len(profileIds) {
				end = len(profileIds)
			}
			if err = s.deleteProfiles(ctx, &sysin.ProfileDeleteInp{Ids: profileIds[start:end]}, tenantId, accountId); err != nil {
				return nil, err
			}
		}
	}
	return res, nil
}

func (s *sSysPublish) collectSourceProfileIds(ctx context.Context, sourceId int64, tenantId int64, accountId int64) ([]int64, error) {
	var rows []struct {
		ProfileId int64 `json:"profileId"`
	}
	err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("DISTINCT profile_id").
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereGT("profile_id", 0).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集源任务失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ProfileId)
	}
	return uniqueIds(ids), nil
}

func (s *sSysPublish) collectSourceDownPreview(ctx context.Context, sourceId int64, tenantId int64, accountId int64, taskIds []int64) (*sysin.CollectSourceDownModel, error) {
	res := &sysin.CollectSourceDownModel{SourceId: sourceId, TaskCount: len(taskIds)}
	if len(taskIds) == 0 {
		return res, nil
	}
	jobCount, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("profile_id", taskIds).
		WhereIn("status", collectSourceDownJobStatuses()).
		Count()
	if err != nil {
		return nil, gerror.Wrap(err, "统计采集源TG任务失败")
	}
	messageCount, err := g.DB().Model(publishTgMessageTable+" m").Safe().Ctx(ctx).
		LeftJoin(publishTgJobTable+" j", "j.id=m.job_id").
		Where("j.tenant_id", tenantId).
		Where("j.account_id", accountId).
		WhereIn("j.profile_id", taskIds).
		Where("m.status", "sent").
		Count()
	if err != nil {
		return nil, gerror.Wrap(err, "统计采集源TG消息失败")
	}
	res.JobCount = jobCount
	res.MessageCount = messageCount
	return res, nil
}

func (s *sSysPublish) collectSourceDownJobs(ctx context.Context, taskIds []int64, tenantId int64) ([]telegramResubmitJob, error) {
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("profile_id", taskIds).
		WhereIn("status", collectSourceDownJobStatuses()).
		OrderAsc("id").
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集源TG任务失败")
	}
	return jobs, nil
}

func (s *sSysPublish) supersedeCollectSourcePendingJobs(ctx context.Context, taskIds []int64, tenantId int64) error {
	if len(taskIds) == 0 {
		return nil
	}
	jobs, err := s.collectSourcePendingDownJobs(ctx, taskIds, tenantId)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}
	jobIds := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		jobIds = append(jobIds, job.Id)
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("id", uniqueIds(jobIds)).
		Data(g.Map{
			"status":              "superseded",
			"dispatch_status":     tgDispatchStatusDone,
			"next_retry_at":       nil,
			"error_message":       collectSourceDownMessage,
			"last_dispatch_error": collectSourceDownMessage,
			"updated_at":          gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "废弃采集源待发送任务失败")
	}
	for _, job := range jobs {
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "down", "superseded", "采集源一键下架，待发送任务已废弃")
	}
	return nil
}

func (s *sSysPublish) collectSourcePendingDownJobs(ctx context.Context, taskIds []int64, tenantId int64) ([]telegramResubmitJob, error) {
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("profile_id", taskIds).
		WhereIn("status", []string{"pending", "failed_retry", "sending", "unknown"}).
		OrderAsc("id").
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集源待发送任务失败")
	}
	return jobs, nil
}

func (s *sSysPublish) finishCollectSourceDownProfiles(ctx context.Context, profileIds []int64, tenantId int64) error {
	if err := s.setProfilesOffline(ctx, profileIds, tenantId); err != nil {
		return err
	}
	return s.refreshProfileNoteIndexes(ctx, profileIds)
}

func (s *sSysPublish) finishCollectSourceDownDispatch(ctx context.Context, sourceId int64, profileIds []int64, tenantId int64, accountId int64) error {
	_, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("profile_id", profileIds).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusSkipped,
			"error_message": collectSourceDownMessage,
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	return gerror.Wrap(err, "更新采集分发下架状态失败")
}

func collectSourceDownJobStatuses() []string {
	return []string{"pending", "sending", "failed_retry", "unknown", "failed", "sent", "superseded"}
}
