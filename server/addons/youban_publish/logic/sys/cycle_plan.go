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
	"hotgo/internal/dao"
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
	_, err := g.DB().Exec(ctx, "UPDATE "+publishChannelProfileTable+" cp SET next_cycle_at=CASE WHEN j.cycle_enabled=1 THEN j.next_cycle_at ELSE NULL END FROM "+publishTgJobTable+" j WHERE j.id=cp.last_job_id AND j.id IN (?) AND cp.tenant_id=j.tenant_id AND cp.account_id=j.account_id AND cp.status='active'", jobIDs)
	return err
}

// repairAccountCyclePlans 独立预算按映射主键小页修复；不扫描历史Job大表。
// 空计划和配置变化与正常到期生产隔离，CAS不覆盖下架、新计划或后续映射。
func (s *sSysPublish) repairAccountCyclePlans(ctx context.Context, scope publishAccountScope) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	key := fmt.Sprintf("%s%d:%d", publishconsts.AccountCycleRepairPrefix, scope.TenantID, scope.AccountID)
	var checkpoint struct {
		Cursor  int64
		Channel int64
		Version int64
		NextAt  int64
	}
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
	if err != nil || channelID <= 0 {
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
	if checkpoint.Channel != channelID || checkpoint.Version != version {
		checkpoint.Channel = channelID
		checkpoint.Version = version
		checkpoint.Cursor = 0
	}
	var page []struct {
		ID        int64
		TenantID  int64       `orm:"tenant_id"`
		AccountID int64       `orm:"account_id"`
		JobID     int64       `orm:"last_job_id"`
		ProfileID int64       `orm:"profile_id"`
		ChannelID int64       `orm:"channel_id"`
		NextAt    *gtime.Time `orm:"next_cycle_at"`
		Version   int64       `orm:"applied_cycle_config_version"`
	}
	if err := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,last_job_id,profile_id,channel_id,next_cycle_at,applied_cycle_config_version").
		Where("status", "active").Where("channel_id", checkpoint.Channel).
		WhereGT("id", checkpoint.Cursor).OrderAsc("id").Limit(20).Scan(&page); err != nil {
		return err
	}
	for _, row := range page {
		if err := ctx.Err(); err != nil {
			return err
		}
		if row.TenantID != scope.TenantID || row.AccountID != scope.AccountID {
			checkpoint.Cursor = row.ID
			continue
		}
		if row.JobID <= 0 {
			g.Log().Warning(ctx, "循环映射缺失成功任务，需要显式恢复", g.Map{"tenantId": scope.TenantID, "accountId": scope.AccountID, "channelId": row.ChannelID, "mappingId": row.ID})
			checkpoint.Cursor = row.ID
			continue
		}
		config, err := s.profileCycleChannelConfigById(ctx, row.ChannelID)
		if err != nil {
			return err
		}
		if !profileCycleChannelUsable(config) || (row.NextAt != nil && row.Version >= config.Version) {
			checkpoint.Cursor = row.ID
			continue
		}
		var job telegramJobRecord
		if err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobID).
			Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).
			Where("channel_id", row.ChannelID).Where("profile_id", row.ProfileID).
			WhereIn("status", []string{"sent", "superseded"}).Scan(&job); err != nil {
			return err
		}
		if job.Id > 0 && job.SentAt != nil && !isDownTelegramOperationNo(job.OperationNo) && !isMessagePushOperationNo(job.OperationNo) {
			nextAt := nextProfileCycleAt(ctx, config.Days, config.PublishTime, job.SentAt, row.ProfileID, row.ChannelID)
			model := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
				Where("id", row.ID).Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).
				Where("status", "active").Where("last_job_id", row.JobID).
				Where("COALESCE(applied_cycle_config_version,0)=?", row.Version).
				Where("EXISTS (SELECT 1 FROM "+dao.ContentProfile.Table()+" p WHERE p.id=? AND p.deleted_at IS NULL AND p.status=1 AND p.visibility='public')", row.ProfileID).
				Where("EXISTS (SELECT 1 FROM "+publishChannelTable+" c WHERE c.id=? AND c.tenant_id=? AND c.deleted_at IS NULL AND c.status=1 AND c.publish_direction='up' AND c.cycle_publish_enabled=1 AND COALESCE(c.cycle_config_version,0)=?)", row.ChannelID, scope.TenantID, config.Version)
			if row.NextAt == nil {
				model = model.WhereNull("next_cycle_at")
			} else {
				model = model.Where("next_cycle_at", row.NextAt)
			}
			res, updateErr := model.Data(g.Map{"next_cycle_at": nextAt, "applied_cycle_config_version": config.Version}).Update()
			if updateErr != nil {
				return updateErr
			}
			affected, updateErr := res.RowsAffected()
			if updateErr != nil {
				return updateErr
			}
			if affected > 0 {
				indexAccountProfileCycleDue(ctx, scope, row.JobID, nextAt)
				if err := s.markAccountSchedule(ctx, scope, true); err != nil {
					g.Log().Warningf(ctx, "唤醒修复循环账户失败 account:%d err:%+v", scope.AccountID, err)
				}
			}
		}
		checkpoint.Cursor = row.ID
	}
	if len(page) < 20 {
		checkpoint.Cursor = 0
		// An enable/config change during this page must retain a fresh rebuild.
		_, err = client.GroupScript().Eval(ctx, `if tonumber(redis.call("HGET",KEYS[2],ARGV[1]) or "0")==tonumber(ARGV[2]) then redis.call("ZREM",KEYS[1],ARGV[1]); redis.call("HDEL",KEYS[2],ARGV[1]); return 1 end return 0`, 2, []string{dirtyKey, dirtyKey + ":versions"}, []interface{}{strconv.FormatInt(checkpoint.Channel, 10), checkpoint.Version})
		if err != nil {
			return err
		}
		checkpoint.Channel = 0
	}
	checkpoint.NextAt = time.Now().Add(10 * time.Second).Unix()
	if cache.Initialized() {
		return cache.Instance().Set(ctx, key, checkpoint, 24*time.Hour)
	}
	return nil
}
