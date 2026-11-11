package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
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
		return s.upsertChannelProfileFromJob(ctx, job)
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

// repairAccountCyclePlans 每次只审计200条成功记录；按账户保存游标，修复丢失映射及空计划，不伪造发送证据。
// 显式下架的inactive映射不恢复；正常路径通过成功事件同步，扫描仅用于迁移和故障补偿。
func (s *sSysPublish) repairAccountCyclePlans(ctx context.Context, scope publishAccountScope) error {
	key := fmt.Sprintf("%s%d:%d", publishconsts.AccountCycleRepairPrefix, scope.TenantID, scope.AccountID)
	value, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return err
	}
	var checkpoint struct {
		Cursor int64
		NextAt int64
	}
	if !value.IsNil() {
		if err := value.Scan(&checkpoint); err != nil {
			return err
		}
	}
	if checkpoint.NextAt > time.Now().Unix() {
		return nil
	}
	// 在数据库读取前写入退避时间，错误页也不能每秒重试；游标仅在整页处理成功后推进。
	checkpoint.NextAt = time.Now().Add(10 * time.Second).Unix()
	if err := cache.Instance().Set(ctx, key, checkpoint, 24*time.Hour); err != nil {
		return err
	}
	var page []struct {
		ID int64 `orm:"id"`
	}
	if err := g.DB().Model(publishTgJobTable).Ctx(ctx).Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).Where("status", "sent").WhereGT("id", checkpoint.Cursor).Fields("id").OrderAsc("id").Limit(200).Scan(&page); err != nil {
		return err
	}
	// 即使本页没有历史记录，也继续检查已有映射的空计划。
	pageIDs := make([]int64, 0, len(page))
	for _, row := range page {
		pageIDs = append(pageIDs, row.ID)
	}
	var jobs []telegramJobRecord
	if len(pageIDs) > 0 {
		if err := g.DB().Model(publishTgJobTable+" j").Ctx(ctx).
			InnerJoin(publishChannelTable+" c", "c.id=j.channel_id AND c.tenant_id=j.tenant_id AND c.deleted_at IS NULL").
			InnerJoin(dao.ContentProfile.Table()+" p", "p.id=j.profile_id AND p.deleted_at IS NULL AND p.status=1 AND p.visibility='public'").
			Where("j.tenant_id", scope.TenantID).Where("j.account_id", scope.AccountID).Where("j.status", "sent").WhereIn("j.id", pageIDs).
			Where("c.cycle_publish_enabled", 1).Where("c.status", 1).Where("c.publish_direction", "up").Where("COALESCE(NULLIF(c.cycle_publish_mode,''),'time')='time'").
			Where("NOT EXISTS(SELECT 1 FROM " + publishChannelProfileTable + " cp WHERE cp.channel_id=j.channel_id AND cp.profile_id=j.profile_id AND (cp.status<>'active' OR cp.next_cycle_at IS NOT NULL OR cp.last_job_id<>j.id))").
			Where("NOT EXISTS(SELECT 1 FROM " + publishTgJobTable + " newer WHERE newer.tenant_id=j.tenant_id AND newer.account_id=j.account_id AND newer.channel_id=j.channel_id AND newer.profile_id=j.profile_id AND newer.status='sent' AND newer.id>j.id)").
			Fields("j.*").OrderAsc("j.id").Limit(200).Scan(&jobs); err != nil {
			return err
		}
	}
	// 已存在映射的空计划优先修复，不必等待历史任务游标绕过整个账户历史。
	var mapped []telegramJobRecord
	if err := g.DB().Model(publishChannelProfileTable+" cp").Ctx(ctx).InnerJoin(publishTgJobTable+" j", "j.id=cp.last_job_id AND j.tenant_id=cp.tenant_id AND j.account_id=cp.account_id AND j.profile_id=cp.profile_id AND j.channel_id=cp.channel_id").InnerJoin(publishChannelTable+" c", "c.id=cp.channel_id AND c.deleted_at IS NULL").Where("cp.tenant_id", scope.TenantID).Where("cp.account_id", scope.AccountID).Where("cp.status", "active").WhereNull("cp.next_cycle_at").Where("c.status", 1).Where("c.cycle_publish_enabled", 1).Where("c.publish_direction", "up").Where("COALESCE(NULLIF(c.cycle_publish_mode,''),'time')='time'").WhereIn("j.status", []string{"sent", "superseded"}).Fields("j.*").OrderAsc("cp.id").Limit(200).Scan(&mapped); err != nil {
		return err
	}
	seen := make(map[int64]bool, len(jobs)+len(mapped))
	for _, job := range mapped {
		seen[job.Id] = true
	}
	for _, job := range jobs {
		if !seen[job.Id] {
			mapped = append(mapped, job)
			seen[job.Id] = true
		}
	}
	jobs = mapped
	// 以真实成功记录为依据；只补不存在的映射，不覆盖后续手工下架或新任务。
	for _, job := range jobs {
		if job.ChannelId <= 0 || job.ProfileId <= 0 || isDownTelegramOperationNo(job.OperationNo) || isMessagePushOperationNo(job.OperationNo) {
			continue
		}
		err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			// 当前资料必须仍属于账户并处于上架状态，历史成功不代表当前仍应发布。
			_, e := tx.Exec("INSERT INTO "+publishChannelProfileTable+" (tenant_id,account_id,channel_id,profile_id,last_job_id,status) SELECT j.tenant_id,j.account_id,j.channel_id,j.profile_id,j.id,'active' FROM "+publishTgJobTable+" j JOIN "+publishProfileStateTable+" ps ON ps.tenant_id=j.tenant_id AND ps.account_id=j.account_id AND ps.profile_id=j.profile_id JOIN "+publishChannelTable+" c ON c.id=j.channel_id AND c.tenant_id=j.tenant_id AND c.deleted_at IS NULL WHERE j.id=? AND ps.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM "+publishTgJobTable+" newer WHERE newer.tenant_id=j.tenant_id AND newer.account_id=j.account_id AND newer.channel_id=j.channel_id AND newer.profile_id=j.profile_id AND newer.status='sent' AND newer.id>j.id) ON CONFLICT(channel_id,profile_id) DO NOTHING", job.Id)
			return e
		})
		if err != nil {
			return err
		}
		// 先补计划再推进审计游标；中途退出会幂等重做本页。
		missing, err := g.DB().Model(publishChannelProfileTable).Ctx(ctx).Where("last_job_id", job.Id).Where("status", "active").WhereNull("next_cycle_at").Count()
		if err != nil {
			return err
		}
		if missing > 0 {
			if err := s.syncTelegramJobCycleSchedule(ctx, job); err != nil {
				return err
			}
		}
	}
	checkpoint.Cursor = 0
	if len(page) == 200 {
		checkpoint.Cursor = page[len(page)-1].ID
	}
	checkpoint.NextAt = time.Now().Add(10 * time.Second).Unix()
	return cache.Instance().Set(ctx, key, checkpoint, 24*time.Hour)
}
