package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	iservice "hotgo/internal/service"
)

func (s *sSysPublish) updateTelegramMediaFileIds(ctx context.Context, messages []*telegramSentMessage) error {
	now := gtime.Now()
	for _, item := range messages {
		if item == nil || item.MediaId <= 0 || item.TgFileId == "" {
			continue
		}
		_, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("id", item.MediaId).
			Data(g.Map{
				"tg_file_id":          item.TgFileId,
				"tg_cache_asset_hash": item.AssetHash,
				"tg_cache_status":     tgCacheStatusValid,
				"updated_at":          now,
			}).
			Update()
		if err != nil {
			return gerror.Wrap(err, "更新TG媒体file_id失败")
		}
	}
	return nil
}

func (s *sSysPublish) incrementDailyPublishStat(ctx context.Context, job telegramJobRecord) error {
	today := gtime.Now().Format("Y-m-d")
	now := gtime.Now()
	result, err := g.DB().Model(publishDailyStatTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("account_id", job.AccountId).
		Where("stat_date", today).
		Increment("published_count", 1)
	if err != nil {
		return gerror.Wrap(err, "更新每日上架统计失败")
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return nil
	}
	_, err = g.DB().Model(publishDailyStatTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":         job.TenantId,
		"account_id":        job.AccountId,
		"stat_date":         today,
		"new_profile_count": 1,
		"published_count":   1,
		"failed_count":      0,
		"down_count":        0,
		"created_at":        now,
		"updated_at":        now,
	}).Insert()
	if err != nil {
		return gerror.Wrap(err, "创建每日上架统计失败")
	}
	return nil
}

func (s *sSysPublish) allTelegramTaskJobsSent(ctx context.Context, taskId int64, operationNo string) (bool, error) {
	if taskId <= 0 {
		return false, nil
	}
	totalMod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		Where("operation_no", operationNo)
	total, err := totalMod.Count()
	if err != nil {
		return false, gerror.Wrap(err, "统计TG任务失败")
	}
	if total == 0 {
		return false, nil
	}
	pending, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		Where("operation_no", operationNo).
		WhereNotIn("status", []string{"sent", "superseded"}).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "统计未完成TG任务失败")
	}
	return pending == 0, nil
}

func (s *sSysPublish) markTaskPublishedAfterTelegram(ctx context.Context, taskId int64, operationNo string) (bool, error) {
	task, err := s.telegramJobTask(ctx, taskId)
	if err != nil {
		return false, err
	}
	profileId := task["profile_id"].Int64()
	now := gtime.Now()
	result, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", taskId).
		Where("status", sysin.PublishTaskStatusPublishing).
		Where("tg_operation_no", operationNo).
		Data(g.Map{
			"status":       sysin.PublishTaskStatusPublished,
			"tg_status":    "sent",
			"published_at": now,
			"updated_at":   now,
		}).
		Update()
	if err != nil {
		return false, gerror.Wrap(err, "更新上架任务发布状态失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	isCycle := isCycleBatchOperation(operationNo)
	if profileId > 0 && !isCycle {
		_, err = s.syncProfilePublishState(ctx, profileId, 1, consts.ContentVisibilityPublic, now)
		if err != nil {
			return false, gerror.Wrap(err, "同步资料上架状态失败")
		}
		if err = s.syncProfileNoteIndex(ctx, profileId); err != nil {
			return false, err
		}
		iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	}
	if !isCycle {
		if err = s.collectFollowProfilePublished(ctx, task); err != nil {
			g.Log().Warningf(ctx, "关注采集发布资料失败 task:%d profile:%d err:%+v", taskId, profileId, err)
		}
	}
	return true, nil
}

func (s *sSysPublish) markPublishedTaskTelegramSent(ctx context.Context, taskId int64, operationNo string) error {
	if taskId <= 0 || operationNo == "" {
		return nil
	}
	_, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", taskId).
		Where("status", sysin.PublishTaskStatusPublished).
		Where("tg_operation_no", operationNo).
		Data(g.Map{
			"tg_status":  "sent",
			"updated_at": gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新循环上架TG状态失败")
	}
	return nil
}
