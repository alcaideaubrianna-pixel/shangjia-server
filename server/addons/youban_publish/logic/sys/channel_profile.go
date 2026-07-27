package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) upsertChannelProfileFromJob(ctx context.Context, job telegramJobRecord) error {
	if job.TenantId <= 0 || job.AccountId <= 0 || job.ChannelId <= 0 || job.ProfileId <= 0 || job.TaskId <= 0 {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(job.OperationNo), "down:") {
		return nil
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id": job.TenantId, "account_id": job.AccountId, "channel_id": job.ChannelId,
		"profile_id": job.ProfileId, "task_id": job.TaskId, "last_job_id": job.Id,
		"status": "active", "updated_at": now,
	}
	result, err := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Where("channel_id", job.ChannelId).
		Where("profile_id", job.ProfileId).
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
		return gerror.Wrap(err, "创建频道当前上架资料索引失败")
	}
	return nil
}

func (s *sSysPublish) backfillChannelProfiles(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 2000
	}
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		InnerJoin(publishTaskTable+" t", "t.id=j.task_id AND t.deleted_at IS NULL").
		Fields("j.id,j.task_id,j.tenant_id,j.account_id,j.profile_id,j.channel_id,j.operation_no").
		Where("j.status", "sent").
		Where("t.status", "published").
		Where("j.profile_id > 0").
		Where("NOT EXISTS (SELECT 1 FROM " + publishChannelProfileTable + " cp WHERE cp.channel_id=j.channel_id AND cp.profile_id=j.profile_id AND cp.status='active')").
		OrderDesc("j.id").
		Limit(limit).
		Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取历史频道上架资料失败")
	}
	seen := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		key := fmt.Sprintf("%d:%d", job.ChannelId, job.ProfileId)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if err = s.upsertChannelProfileFromJob(ctx, job); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) channelProfileBackfillPending(ctx context.Context) (bool, error) {
	count, err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		InnerJoin(publishTaskTable+" t", "t.id=j.task_id AND t.deleted_at IS NULL").
		Where("j.status", "sent").
		Where("t.status", "published").
		Where("j.profile_id > 0").
		Where("NOT EXISTS (SELECT 1 FROM " + publishChannelProfileTable + " cp WHERE cp.channel_id=j.channel_id AND cp.profile_id=j.profile_id AND cp.status='active')").
		Limit(1).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查历史频道资料迁移进度失败")
	}
	return count > 0, nil
}

func (s *sSysPublish) syncPublishedTaskChannelProfiles(ctx context.Context, taskId int64) error {
	task, err := s.telegramJobTask(ctx, taskId)
	if err != nil {
		return err
	}
	profileId := task["profile_id"].Int64()
	if profileId <= 0 {
		return nil
	}
	channelIds := decodeInt64JSON(task["channel_id_json"].String())
	mod := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Where("tenant_id", task["tenant_id"].Int64()).
		Where("profile_id", profileId).
		Where("status", "active")
	if len(channelIds) > 0 {
		mod = mod.WhereNotIn("channel_id", channelIds)
	}
	_, err = mod.Data(g.Map{"status": "inactive", "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "同步资料当前上架频道失败")
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
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", current.TenantId).
		Where("profile_id", current.ProfileId).
		Where("channel_id", current.ChannelId).
		Where("status", "sent").
		WhereNot("id", current.Id).
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
