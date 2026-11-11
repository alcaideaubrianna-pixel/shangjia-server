package sys

import (
	"context"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const publishChannelProfileTable = "hg_youban_publish_channel_profile"

func (s *sSysPublish) syncChannelCycleAfterSave(ctx context.Context, tenantId int64, channelId int64, enabled int, days int, publishTime string) error {
	if tenantId <= 0 || channelId <= 0 {
		return nil
	}
	data := g.Map{
		"cycle_active_run_id":      0,
		"cycle_last_error_message": "",
		"cycle_publish_enabled":    enabled,
		"cycle_publish_days":       days,
		"cycle_publish_time":       strings.TrimSpace(publishTime),
		"updated_at":               gtime.Now(),
		"cycle_scheduler_shard":    gdb.Raw("MOD(ABS(HASHTEXT(tenant_id::text || ':' || id::text)),64)"),
	}
	data["cycle_next_run_at"] = nil
	_, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("id", channelId).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新频道循环上架时间失败")
	}
	if err = s.enqueueCycleReschedule(ctx, channelId, 0); err != nil {
		g.Log().Warningf(ctx, "提交频道循环重算任务失败，定时调度将自动恢复 channel:%d err:%+v", channelId, err)
	}
	// 配置修改必须立即投影到频道内已有资料，否则资料仍会沿用旧循环时间或旧开关。
	var jobs []telegramJobRecord
	if err = g.DB().Model(publishChannelProfileTable+" cp").Safe().Ctx(ctx).
		InnerJoin(publishTgJobTable+" j", "j.id=cp.last_job_id AND j.tenant_id=cp.tenant_id AND j.account_id=cp.account_id AND j.channel_id=cp.channel_id AND j.profile_id=cp.profile_id").
		Where("cp.tenant_id", tenantId).Where("cp.channel_id", channelId).Where("cp.status", "active").
		WhereIn("j.status", []string{"sent", "superseded"}).Fields("j.*").Limit(profileCycleOrchestratorBatchSize(ctx)).Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取频道循环资料计划失败")
	}
	for _, job := range jobs {
		if err = s.syncTelegramJobCycleSchedule(ctx, job); err != nil {
			return gerror.Wrapf(err, "同步频道资料循环计划失败 profileId:%d", job.ProfileId)
		}
	}
	// 频道配置变化后立即唤醒关联资料级 run，避免继续等待旧的到期时间。
	if err = s.wakeProfileCycleRunsForChannel(ctx, tenantId, channelId); err != nil {
		g.Log().Warningf(ctx, "唤醒频道资料循环任务失败 channel:%d err:%+v", channelId, err)
	}
	return nil
}

// wakeProfileCycleRunsForChannel 将受配置影响的 pending run 提前到当前时间。
// run 仍按资料聚合，因此只更新关联频道的资料 run，不创建重复 run。
func (s *sSysPublish) wakeProfileCycleRunsForChannel(ctx context.Context, tenantID, channelID int64) error {
	if tenantID <= 0 || channelID <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishProfileCycleRunTable).Ctx(ctx).Where("tenant_id", tenantID).Where("status", "pending").
		Where("EXISTS (SELECT 1 FROM "+publishProfileCycleRunChannelTable+" rc WHERE rc.run_id="+publishProfileCycleRunTable+".id AND rc.channel_id=?)", channelID).
		Data(g.Map{"due_at": gtime.Now()}).Update()
	return err
}

func parseCycleClock(value string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) < 2 {
		return 0, 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}
