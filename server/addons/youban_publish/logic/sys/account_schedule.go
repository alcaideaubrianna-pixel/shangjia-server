package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
	"hotgo/utility/ownedlease"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
)

type publishAccountScope struct {
	TenantID  int64 `json:"tenantId"`
	AccountID int64 `json:"accountId"`
}

// pauseInactiveAccountCycles 复用免费账户保护策略，每账户每分钟检查一次，避免每秒重复遍历全部频道。
func (s *sSysPublish) pauseInactiveAccountCycles(ctx context.Context, scope publishAccountScope) error {
	key := fmt.Sprintf("%s%d:%d", publishconsts.AccountScheduleMaintenancePrefix, scope.TenantID, scope.AccountID)
	value, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return err
	}
	if !value.IsNil() {
		return nil
	}
	if err := s.pauseInactiveFreeChannelCycles(ctx, scope); err != nil {
		return err
	}
	return cache.Instance().Set(ctx, key, true, time.Minute)
}

// syncProfileCycleRunJob 频道独立保存循环结果；只有最后一个频道完成时收口run，不在发送前获取资料锁。
func (s *sSysPublish) syncProfileCycleRunJob(ctx context.Context, job telegramJobRecord) error {
	if !strings.HasPrefix(job.OperationNo, "profile_cycle:") {
		return nil
	}
	value, err := g.DB().Model(publishProfileCycleRunChannelTable).Ctx(ctx).Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).Where("job_id", job.Id).Fields("run_id").Value()
	if err != nil || value.IsNil() {
		return err
	}
	runID := value.Int64()
	if _, err := g.DB().Model(publishProfileCycleRunChannelTable).Ctx(ctx).Where("run_id", runID).Where("job_id", job.Id).Data(g.Map{"status": "sent"}).Update(); err != nil {
		return err
	}
	// 先提交本频道结果，再检查全体，避免并发事务互相看不到对方的最终状态。
	return finishProfileCycleRun(ctx, runID)
}

// finishProfileCycleRun 统一收口成功、部分失败、全部失败和跳过的频道结果，不把失败当成仍在排队。
func finishProfileCycleRun(ctx context.Context, runID int64) error {
	_, err := g.DB().Exec(ctx, "UPDATE "+publishProfileCycleRunTable+" r SET status=CASE WHEN EXISTS(SELECT 1 FROM "+publishProfileCycleRunChannelTable+" f WHERE f.run_id=r.id AND f.status='failed') THEN CASE WHEN EXISTS(SELECT 1 FROM "+publishProfileCycleRunChannelTable+" ok WHERE ok.run_id=r.id AND ok.status='sent') THEN 'partial' ELSE 'failed' END ELSE 'completed' END,completed_at=NOW() WHERE r.id=? AND r.status='dispatched' AND NOT EXISTS(SELECT 1 FROM "+publishProfileCycleRunChannelTable+" rc WHERE rc.run_id=r.id AND rc.status NOT IN ('sent','superseded','failed','skipped'))", runID)
	return err
}

// cycleRunAccountModel 通过所属频道限定批次运行的账户，避免恢复和收尾扫描全局批次。
func cycleRunAccountModel(ctx context.Context, scopes []publishAccountScope) *gdb.Model {
	model := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx)
	if len(scopes) > 0 {
		model = model.Where("tenant_id", scopes[0].TenantID).Where("EXISTS (SELECT 1 FROM "+publishChannelTable+" owner WHERE owner.id="+publishCycleRunTable+".channel_id AND owner.tg_account_id=?)", scopes[0].AccountID)
	}
	return model
}

// accountScheduleModel 将扫描限定到一个账户；旧全局扫描只保留未迁移账户。
func accountScheduleModel(model *gdb.Model, tenantField, accountField string, scopes []publishAccountScope) *gdb.Model {
	if len(scopes) > 0 {
		return model.Where(tenantField, scopes[0].TenantID).Where(accountField, scopes[0].AccountID)
	}
	return model.Where("NOT EXISTS (SELECT 1 FROM " + publishAccountSettingTable + " se WHERE se.tenant_id=" + tenantField + " AND se.account_id=" + accountField + " AND se.deleted_at IS NULL AND se.push_engine='unified_stream')")
}

// enqueueAccountSchedules 只按主键分页读取账户目录，不扫描全局任务大表；复用已有后台队列。
func (s *sSysPublish) enqueueAccountSchedules(ctx context.Context) error {
	cursor, err := cache.Instance().Get(ctx, publishconsts.AccountScheduleCursorKey)
	if err != nil {
		return err
	}
	var accounts []struct {
		ID       int64 `orm:"id"`
		TenantID int64 `orm:"tenant_id"`
	}
	if err := g.DB().Model(publishAccountTable).Ctx(ctx).Fields("id,tenant_id").WhereGT("id", cursor.Int64()).Where("status", 1).
		OrderAsc("id").Limit(100).Scan(&accounts); err != nil {
		return err
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	for _, account := range accounts {
		if s.accountPushEngine(ctx, account.TenantID, account.ID) != pushEngineUnifiedStream {
			continue
		}
		body, err := json.Marshal(publishAccountScope{TenantID: account.TenantID, AccountID: account.ID})
		if err != nil {
			return err
		}
		_, err = client.EnqueueContext(ctx, asynq.NewTask(publishconsts.QueueAccountScheduleTask, body), asynq.Queue(tgQueueNameBackground),
			asynq.Unique(2*time.Second), asynq.Timeout(25*time.Second), asynq.MaxRetry(3))
		if err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			return err
		}
	}
	next := int64(0)
	if len(accounts) == 100 {
		next = accounts[len(accounts)-1].ID
	}
	return cache.Instance().Set(ctx, publishconsts.AccountScheduleCursorKey, next, time.Hour)
}

// handleAccountScheduleTask 对每个账户独立限时、独立租约；异常账户不占全局调度锁。
func (s *sSysPublish) handleAccountScheduleTask(ctx context.Context, task *asynq.Task) error {
	var scope publishAccountScope
	if err := json.Unmarshal(task.Payload(), &scope); err != nil {
		return err
	}
	if scope.TenantID <= 0 || scope.AccountID <= 0 {
		return asynq.SkipRetry
	}
	if s.accountPushEngine(ctx, scope.TenantID, scope.AccountID) != pushEngineUnifiedStream {
		return nil
	}
	lease := ownedlease.New(fmt.Sprintf("%s%d:%d", publishconsts.AccountScheduleLeasePrefix, scope.TenantID, scope.AccountID), "account_schedule", telegramStreamExecutionTTL)
	ok, err := lease.TryAcquire(ctx)
	if err != nil || !ok {
		return err
	}
	bounded, stop := telegramStreamExecutionContext(ctx, lease)
	defer func() { stop(); s.releaseTelegramChannelLease(ctx, lease) }()
	// 各阶段都独立继续；错误交给Asynq重试，单阶段不能吞掉其他入口。
	var failures []error
	for stageIndex, stage := range []func(context.Context) error{
		func(ctx context.Context) error { return s.submitDuePublishTasks(ctx, scope) },
		func(ctx context.Context) error { return s.pauseInactiveAccountCycles(ctx, scope) },
		func(ctx context.Context) error { return s.produceAccountProfileCycles(ctx, scope) },
		func(ctx context.Context) error { return s.runProfileCycleOrchestrator(ctx, scope) },
		func(ctx context.Context) error { return s.scheduleDueChannelBatchCyclesShard(ctx, 20, -1, scope) },
		func(ctx context.Context) error { return s.finalizeDispatchingChannelCycleRuns(ctx, 20, scope) },
		func(ctx context.Context) error { return s.recoverChannelCycleRuns(ctx, scope) },
		// 历史映射修复是低优先级补偿，必须放在当前循环生产之后，不能阻塞用户当前曝光。
		func(ctx context.Context) error { return s.repairAccountCyclePlans(ctx, scope) },
	} {
		// 阶段各自限时，慢查询不能耗尽账户全部预算，挤掉后续循环与恢复。
		stageCtx, cancel := context.WithTimeout(bounded, 2*time.Second)
		err := stage(stageCtx)
		budgetEnded := stageCtx.Err() != nil && bounded.Err() == nil
		cancel()
		if err != nil && budgetEnded {
			g.Log().Info(ctx, "账户调度达到本轮预算，剩余持久计划下轮继续", g.Map{"tenantId": scope.TenantID, "accountId": scope.AccountID, "stage": stageIndex + 1})
			continue
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// produceAccountProfileCycles 复用已成功任务的到期时间；一份资料的一轮只创建一个run和一次频道准备。
func (s *sSysPublish) produceAccountProfileCycles(ctx context.Context, scope publishAccountScope) error {
	// 复用已有调度批量ENV，不再把单账户固定限制为每次20份资料。
	limit := profileCycleOrchestratorBatchSize(ctx)
	rows, err := s.profileCycleDueRowsShard(ctx, limit, -1, scope)
	if err != nil {
		return err
	}
	seen := make(map[int64]bool)
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if seen[row.ProfileId] {
			continue
		}
		seen[row.ProfileId] = true
		if err := s.createAccountProfileCycle(ctx, scope, row); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			g.Log().Warningf(ctx, "创建账户资料循环失败 tenant:%d account:%d profile:%d err:%+v", scope.TenantID, scope.AccountID, row.ProfileId, err)
			continue
		}
		if len(seen) >= limit {
			break
		}
	}
	return nil
}

// createAccountProfileCycle 使用资料行锁和稳定run_key防止并发重复创建，保留频道各自的到期时间。
func (s *sSysPublish) createAccountProfileCycle(ctx context.Context, scope publishAccountScope, row profileCycleDueRow) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model(publishProfileStateTable).Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).Where("profile_id", row.ProfileId).LockUpdate().One(); err != nil {
			return err
		}
		// 领取时重读计划并锁定本条映射；扫描后已被手工上架替换或延期的计划不能继续执行。
		var current struct {
			Generation int         `orm:"cycle_generation"`
			DueAt      *gtime.Time `orm:"next_cycle_at"`
		}
		if err := tx.Model(publishChannelProfileTable).Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).Where("profile_id", row.ProfileId).Where("last_job_id", row.JobId).Where("status", "active").WhereLTE("next_cycle_at", gtime.Now()).LockUpdate().Fields("cycle_generation,next_cycle_at").Scan(&current); err != nil {
			return err
		}
		if current.DueAt == nil {
			return nil
		}
		row.DispatchCount = current.Generation
		row.NextCycleAt = current.DueAt
		// 只把仍在执行的运行视为屏障；终态运行由同一检查顺手收口。
		if _, err := tx.Exec("UPDATE "+publishProfileCycleRunTable+" r SET status=CASE WHEN EXISTS(SELECT 1 FROM "+publishProfileCycleRunChannelTable+" rc JOIN "+publishTgJobTable+" j ON j.id=rc.job_id WHERE rc.run_id=r.id AND j.status NOT IN ('sent','superseded')) THEN 'failed' ELSE 'completed' END,completed_at=NOW() WHERE r.tenant_id=? AND r.account_id=? AND r.profile_id=? AND r.status='dispatched' AND NOT EXISTS(SELECT 1 FROM "+publishProfileCycleRunChannelTable+" rc JOIN "+publishTgJobTable+" j ON j.id=rc.job_id WHERE rc.run_id=r.id AND j.status IN ('pending','sending','failed_retry','unknown'))", scope.TenantID, scope.AccountID, row.ProfileId); err != nil {
			return err
		}
		// 仅防止重复创建尚未准备任务的同一轮；发送中的频道不形成资料级屏障。
		pending, err := tx.Model(publishProfileCycleRunTable).Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).Where("profile_id", row.ProfileId).Where("status", "pending").Count()
		if err != nil || pending > 0 {
			return err
		}
		// 不设置资料级全频道屏障：一个频道异常不能阻止其他频道进入下一轮。
		var channels []struct {
			ID int64 `orm:"channel_id"`
		}
		if err := tx.Model(publishTgJobTable+" j").InnerJoin(publishChannelProfileTable+" cp", "cp.last_job_id=j.id AND cp.status='active'").
			InnerJoin(publishChannelTable+" c", "c.id=j.channel_id AND c.deleted_at IS NULL").
			Where("j.tenant_id", scope.TenantID).Where("j.account_id", scope.AccountID).Where("j.profile_id", row.ProfileId).
			WhereNotNull("cp.next_cycle_at").WhereLTE("cp.next_cycle_at", gtime.Now()).WhereIn("j.status", []string{"sent", "superseded"}).
			Where("c.status", 1).Where("c.cycle_publish_enabled", 1).Where("COALESCE(NULLIF(LOWER(TRIM(c.cycle_publish_mode)),''),'time')='time'").Where("c.publish_direction", "up").
			Where("NOT EXISTS (SELECT 1 FROM " + publishTgJobTable + " running WHERE running.tenant_id=j.tenant_id AND running.account_id=j.account_id AND running.profile_id=j.profile_id AND running.channel_id=j.channel_id AND running.status IN ('pending','sending','failed_retry','unknown'))").Fields("j.channel_id").Scan(&channels); err != nil {
			return err
		}
		if len(channels) == 0 {
			return nil
		}
		key := fmt.Sprintf("account:%d:%d:%d:plan:%d", scope.AccountID, row.ProfileId, row.JobId, row.DispatchCount+1)
		id, err := tx.Model(publishProfileCycleRunTable).Data(g.Map{"tenant_id": scope.TenantID, "account_id": scope.AccountID, "profile_id": row.ProfileId, "run_key": key, "status": "pending", "due_at": row.NextCycleAt}).InsertAndGetId()
		if err != nil {
			return err
		}
		for _, channel := range channels {
			if _, err := tx.Model(publishProfileCycleRunChannelTable).Data(g.Map{"run_id": id, "tenant_id": scope.TenantID, "account_id": scope.AccountID, "profile_id": row.ProfileId, "channel_id": channel.ID, "status": "pending"}).Insert(); err != nil {
				return err
			}
		}
		// 只推进轻量计划的领取代次与重试时间，不改写历史发送任务的调度计数。
		_, err = tx.Model(publishChannelProfileTable).Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).Where("profile_id", row.ProfileId).Where("last_job_id", row.JobId).
			Data(g.Map{"cycle_generation": gdb.Raw("cycle_generation+1"), "next_cycle_at": gtime.Now().Add(profileCycleRetryDelay)}).Update()
		return err
	})
}
