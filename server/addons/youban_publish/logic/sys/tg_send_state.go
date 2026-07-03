package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
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
			Where("tg_file_id", "").
			Data(g.Map{"tg_file_id": item.TgFileId, "updated_at": now}).
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

func (s *sSysPublish) allTelegramTaskJobsSent(ctx context.Context, taskId int64) (bool, error) {
	if taskId <= 0 {
		return false, nil
	}
	total, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "统计TG任务失败")
	}
	if total == 0 {
		return false, nil
	}
	pending, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		WhereNotIn("status", []string{"sent", "superseded"}).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "统计未完成TG任务失败")
	}
	return pending == 0, nil
}

func (s *sSysPublish) markTaskPublishedAfterTelegram(ctx context.Context, taskId int64) (bool, error) {
	task, err := s.telegramJobTask(ctx, taskId)
	if err != nil {
		return false, err
	}
	profileId := task["profile_id"].Int64()
	now := gtime.Now()
	result, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", taskId).
		WhereNot("status", sysin.PublishTaskStatusPublished).
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
	if profileId > 0 {
		profileColumns := dao.ContentProfile.Columns()
		_, err = dao.ContentProfile.Ctx(ctx).
			Where(profileColumns.Id, profileId).
			Data(g.Map{
				profileColumns.Status:      1,
				profileColumns.Visibility:  consts.ContentVisibilityPublic,
				profileColumns.PublishedAt: now,
				profileColumns.UpdatedAt:   now,
			}).
			Update()
		if err != nil {
			return false, gerror.Wrap(err, "同步资料上架状态失败")
		}
		iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}
