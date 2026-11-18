package sys

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
)

// confirmTelegramJobAndCyclePlan 将成功状态、当前映射和旧计划失效原子提交；事务内只访问数据库。
// 写入失败保留已确认Attempt，由原对账链路恢复，不能重新调用Telegram发送。
func (s *sSysPublish) confirmTelegramJobAndCyclePlan(ctx context.Context, job telegramJobRecord, transition telegramJobTransition) (changed bool, err error) {
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		changed, err = s.transitionTelegramJobTx(ctx, tx, transition)
		if err != nil || !changed {
			return err
		}
		if _, err = s.closeRecoveredTelegramJobTx(ctx, tx, job.Id); err != nil {
			return err
		}
		return s.upsertChannelProfileFromJobTx(ctx, tx, job)
	})
	return
}

// projectChannelCyclePlans 将指定成功任务的计划投影到当前频道资料映射；旧任务不能覆盖新映射。
// Job继续保存历史兼容字段，新引擎只从轻量映射的到期索引选择计划。
func projectChannelCyclePlans(ctx context.Context, jobIDs []int64) error {
	if len(jobIDs) == 0 {
		return nil
	}
	_, err := g.DB().Exec(ctx, "UPDATE "+publishChannelProfileTable+" cp SET next_cycle_at=CASE WHEN j.cycle_enabled=1 THEN j.next_cycle_at ELSE NULL END,applied_profile_cycle_version=COALESCE(j.profile_cycle_version,0) FROM "+publishTgJobTable+" j WHERE j.id=cp.last_job_id AND j.id IN (?) AND cp.tenant_id=j.tenant_id AND cp.account_id=j.account_id AND cp.status='active' AND COALESCE(cp.applied_profile_cycle_version,0)<=COALESCE(j.profile_cycle_version,0)", jobIDs)
	return err
}

// repairAccountCyclePlans 独立预算按映射主键小页修复；不扫描历史Job大表。
// 空计划和配置变化与正常到期生产隔离，CAS不覆盖下架、新计划或后续映射。
func (s *sSysPublish) repairAccountCyclePlans(ctx context.Context, scope publishAccountScope) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	key := fmt.Sprintf("%s%d:%d", publishconsts.AccountCycleRepairPrefix, scope.TenantID, scope.AccountID)
	var checkpoint profileCycleRepairCheckpoint
	if cache.Initialized() {
		if v, err := cache.Instance().Get(ctx, key); err == nil && !v.IsNil() {
			_ = v.Scan(&checkpoint)
		}
	}
	if checkpoint.NextAt > time.Now().Unix() {
		return nil
	}
	// 仅消费本账户显式dirty频道；禁止以租户目录回退扫描无索引空计划。
	dirtyKey := accountProfileCycleDueKey(scope) + ":repair_channels"
	if !cache.Initialized() {
		return nil
	}
	members, err := cache.ZSetRange(ctx, dirtyKey, 0, 0)
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return nil
	}
	channelID, err := strconv.ParseInt(members[0], 10, 64)
	if err != nil || channelID == 0 {
		_ = cache.ZSetRemove(ctx, dirtyKey, members[0])
		return nil
	}
	client, err := cache.RedisClient()
	if err != nil {
		return err
	}
	versionValue, err := client.GroupHash().HGet(ctx, dirtyKey+":versions", members[0])
	if err != nil {
		return err
	}
	version := versionValue.Int64()
	checkpoint.selectTarget(channelID, version)
	if channelID < 0 {
		profileVersion, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
			Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).
			Where("profile_id", -channelID).WhereNull("deleted_at").Value("cycle_override_version")
		if err != nil {
			return err
		}
		checkpoint.selectProfileVersion(profileVersion.Int64())
	} else {
		checkpoint.ProfileVersion = 0
	}
	var page []struct {
		ID             int64
		TenantID       int64       `orm:"tenant_id"`
		AccountID      int64       `orm:"account_id"`
		JobID          int64       `orm:"last_job_id"`
		ProfileID      int64       `orm:"profile_id"`
		ChannelID      int64       `orm:"channel_id"`
		NextAt         *gtime.Time `orm:"next_cycle_at"`
		Version        int64       `orm:"applied_cycle_config_version"`
		ProfileVersion int64       `orm:"applied_profile_cycle_version"`
	}
	model := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,last_job_id,profile_id,channel_id,next_cycle_at,applied_cycle_config_version,applied_profile_cycle_version").
		Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).Where("status", "active")
	if checkpoint.Channel < 0 {
		model = model.Where("profile_id", -checkpoint.Channel)
	} else {
		model = model.Where("channel_id", checkpoint.Channel)
	}
	if err := model.WhereGT("id", checkpoint.Cursor).OrderAsc("id").Limit(20).Scan(&page); err != nil {
		return err
	}
	batch := make([]profileCycleRescheduleRow, 0, len(page))
	for _, row := range page {
		batch = append(batch, profileCycleRescheduleRow{
			RelationId: row.ID, TenantId: scope.TenantID, AccountId: scope.AccountID,
			JobId: row.JobID, ProfileId: row.ProfileID, ChannelId: row.ChannelID,
			NextAt: row.NextAt, AppliedChannelVersion: row.Version, AppliedProfileVersion: row.ProfileVersion,
			PreserveCurrent: checkpoint.Channel > 0,
		})
	}
	if err := s.updateProfileCycleRescheduleBatch(ctx, profileCycleChannelConfig{}, batch, gtime.Now()); err != nil {
		return err
	}
	if len(page) > 0 {
		checkpoint.Cursor = page[len(page)-1].ID
		if err := cache.Instance().Set(ctx, key, checkpoint, 24*time.Hour); err != nil {
			return err
		}
	}
	if len(page) < 20 {
		checkpoint.Cursor = 0
		// An enable/config change during this page must retain a fresh rebuild.
		_, err = client.GroupScript().Eval(ctx, `if (redis.call("HGET",KEYS[2],ARGV[1]) or "0")==ARGV[2] then redis.call("ZREM",KEYS[1],ARGV[1]); redis.call("HDEL",KEYS[2],ARGV[1]); return 1 end return 0`, 2, []string{dirtyKey, dirtyKey + ":versions"}, []interface{}{strconv.FormatInt(checkpoint.Channel, 10), checkpoint.Version})
		if err != nil {
			return err
		}
		checkpoint.Channel = 0
	}
	checkpoint.NextAt = time.Now().Add(10 * time.Second).Unix()
	if err := cache.Instance().Set(ctx, key, checkpoint, 24*time.Hour); err != nil {
		return err
	}
	// Keep the existing account consumer alive until later pages (or the next
	// dirty target) have been consumed. The checkpoint is committed first.
	if err := scheduleAccountCycleWake(ctx, scope, time.Unix(checkpoint.NextAt, 0)); err != nil {
		return err
	}
	return errAccountScheduleMore
}
