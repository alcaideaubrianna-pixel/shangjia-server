package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) upsertChannelProfileFromJob(ctx context.Context, job telegramJobRecord) error {
	if job.TenantId <= 0 || job.AccountId <= 0 || job.ChannelId <= 0 || job.ProfileId <= 0 {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(job.OperationNo), "down:") {
		return nil
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id": job.TenantId, "account_id": job.AccountId, "channel_id": job.ChannelId,
		"profile_id": job.ProfileId, "last_job_id": job.Id,
		"status": "active", "updated_at": now,
	}
	result, err := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Where("channel_id", job.ChannelId).
		Where("profile_id", job.ProfileId).
		Where("last_job_id <= ?", job.Id).
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新频道当前上架资料索引失败")
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return nil
	}
	data["created_at"] = now
	if _, err = g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
		if !isDuplicateKeyError(err) {
			return gerror.Wrap(err, "创建频道当前上架资料索引失败")
		}
		_, err = g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
			Where("channel_id", job.ChannelId).
			Where("profile_id", job.ProfileId).
			Where("last_job_id <= ?", job.Id).
			Data(data).
			Update()
		if err != nil {
			return gerror.Wrap(err, "并发更新频道当前上架资料索引失败")
		}
	}
	return nil
}

func (s *sSysPublish) deactivateChannelProfile(ctx context.Context, channelId int64, profileId int64) error {
	if channelId <= 0 || profileId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Where("channel_id", channelId).
		Where("profile_id", profileId).
		Data(g.Map{"status": "inactive", "updated_at": gtime.Now()}).
		Update()
	return err
}

func (s *sSysPublish) deactivateChannelProfiles(ctx context.Context, tenantId int64, profileIds []int64) error {
	profileIds = uniqueIds(profileIds)
	if tenantId <= 0 || len(profileIds) == 0 {
		return nil
	}
	_, err := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("profile_id", profileIds).
		Data(g.Map{"status": "inactive", "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "停用频道当前上架资料索引失败")
	}
	return nil
}

func (s *sSysPublish) cleanupPreviousCycleMessages(ctx context.Context, current telegramJobRecord) {
	latest, err := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Fields("last_job_id").
		Where("channel_id", current.ChannelId).
		Where("profile_id", current.ProfileId).
		Where("status", "active").
		One()
	if err != nil {
		g.Log().Warningf(ctx, "读取循环上架最新频道索引失败 job:%d err:%+v", current.Id, err)
		return
	}
	if latest["last_job_id"].Int64() != current.Id {
		return
	}
	var jobs []telegramJobRecord
	err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", current.TenantId).
		Where("profile_id", current.ProfileId).
		Where("channel_id", current.ChannelId).
		Where("status", "sent").
		WhereLT("id", current.Id).
		OrderAsc("id").
		Scan(&jobs)
	if err != nil {
		g.Log().Warningf(ctx, "读取循环上架旧消息失败 job:%d err:%+v", current.Id, err)
		return
	}
	for _, job := range jobs {
		if err = s.deleteTelegramMessageSetLockedByChannel(ctx, job, "频道循环上架"); err != nil {
			s.appendTelegramJobLog(ctx, job, "cycle_delete", "retry", "循环上架删除旧消息失败，已加入清理队列："+err.Error())
			_ = s.enqueueTelegramCleanupJob(ctx, job.Id, 0)
			continue
		}
		_ = s.markTelegramJobSuperseded(ctx, job.Id)
	}
}
