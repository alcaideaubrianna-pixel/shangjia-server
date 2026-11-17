package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) upsertChannelProfileFromJob(ctx context.Context, job telegramJobRecord) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return s.upsertChannelProfileFromJobTx(ctx, tx, job)
	})
}

// upsertChannelProfileFromJobTx 在调用方事务中写入频道资料映射，避免事务内再次申请数据库连接。
func (s *sSysPublish) upsertChannelProfileFromJobTx(ctx context.Context, tx gdb.TX, job telegramJobRecord) error {
	if job.TenantId <= 0 || job.AccountId <= 0 || job.ChannelId <= 0 || job.ProfileId <= 0 {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(job.OperationNo), "down:") {
		return nil
	}
	now := gtime.Now()
	// Use one PostgreSQL upsert instead of UPDATE -> INSERT -> UPDATE. The old
	// sequence raised a duplicate-key error under concurrency, which aborted the
	// transaction; the fallback UPDATE then only produced current transaction is
	// aborted and caused confirmed Telegram jobs to retry forever.
	sql := "INSERT INTO " + publishChannelProfileTable +
		" (tenant_id,account_id,channel_id,profile_id,last_job_id,status,next_cycle_at,created_at,updated_at)" +
		" VALUES (?,?,?,?,?,'active',NULL,?,?)" +
		" ON CONFLICT (channel_id,profile_id) DO UPDATE SET" +
		" tenant_id=EXCLUDED.tenant_id,account_id=EXCLUDED.account_id,last_job_id=EXCLUDED.last_job_id," +
		" status='active',next_cycle_at=NULL,updated_at=EXCLUDED.updated_at" +
		" WHERE " + publishChannelProfileTable + ".last_job_id <= EXCLUDED.last_job_id"
	if _, err := tx.Ctx(ctx).Exec(sql,
		job.TenantId, job.AccountId, job.ChannelId, job.ProfileId, job.Id, now, now,
	); err != nil {
		return gerror.Wrap(err, "保存频道当前上架资料索引失败")
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
		Where("target_chat_id", normalizeTelegramChannelChatID(current.TargetChatId)).
		WhereIn("status", []string{"sent", "superseded"}).
		Where("EXISTS (SELECT 1 FROM "+publishTgMessageTable+" m WHERE m.job_id="+publishTgJobTable+".id AND m.status='sent')").
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
