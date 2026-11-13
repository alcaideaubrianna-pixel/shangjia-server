package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
	"hotgo/utility/ownedlease"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
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
	// 循环频道投影只接受 Telegram 任务终态，发送中或重试中的任务不能提前收口。
	if !strings.HasPrefix(job.OperationNo, "profile_cycle:") {
		return nil
	}
	// Outbox 尚未完成时不能收口投影；兼容历史任务的空 dispatch_status。
	if job.DispatchStatus != "" && job.DispatchStatus != tgDispatchStatusDone {
		return nil
	}
	projectionStatus := ""
	switch job.Status {
	case telegramJobStatusSent:
		projectionStatus = "sent"
	case telegramJobStatusSuperseded:
		projectionStatus = "superseded"
	case telegramJobStatusFailed:
		projectionStatus = "failed"
	default:
		return nil
	}
	value, err := g.DB().Model(publishProfileCycleRunChannelTable).Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).Where("job_id", job.Id).
		Fields("run_id").Value()
	if err != nil || value.IsNil() {
		return err
	}
	runID := value.Int64()
	if runID <= 0 {
		return nil
	}
	if _, err := g.DB().Model(publishProfileCycleRunChannelTable).Ctx(ctx).
		Where("run_id", runID).Where("job_id", job.Id).
		Data(g.Map{"status": projectionStatus, "error_message": job.ErrorMessage, "updated_at": gtime.Now()}).
		Update(); err != nil {
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
	// 账户目录游标只保证账户轮询公平，不能覆盖长期到期的资料循环。
	// 直接从账户目录反查到期资料，避免 DISTINCT 扫描三百万行
	// channel_profile。每个账户通过 EXISTS 命中
	// (tenant_id,account_id,next_cycle_at,id) 索引；游标仍按账户主键推进，
	// 因此每分钟扫描只触碰当前批次账户。
	dueAccounts := make([]struct {
		ID       int64 `orm:"id"`
		TenantID int64 `orm:"tenant_id"`
	}, 0)
	if err := g.DB().Model(publishAccountTable+" a").Ctx(ctx).
		Fields("a.id,a.tenant_id").
		Where("a.status", 1).
		Where("a.deleted_at IS NULL").
		Where(fmt.Sprintf(`EXISTS (
			SELECT 1
			FROM %s cp
			INNER JOIN %s c
				ON c.id=cp.channel_id AND c.tenant_id=cp.tenant_id
			WHERE cp.tenant_id=a.tenant_id
			  AND cp.account_id=a.id
			  AND cp.status='active'
			  AND cp.next_cycle_at IS NOT NULL
			  AND cp.next_cycle_at<=?
			  AND c.status=1
			  AND c.cycle_publish_enabled=1
			  AND c.publish_direction='up'
			  AND c.deleted_at IS NULL
		)`, publishChannelProfileTable, publishChannelTable), gtime.Now()).
		OrderAsc("a.id").Limit(100).Scan(&dueAccounts); err != nil {
		return err
	}
	seenAccounts := make(map[int64]struct{}, len(accounts)+len(dueAccounts))
	for _, account := range accounts {
		seenAccounts[account.TenantID*1000000000+account.ID] = struct{}{}
	}
	for _, account := range dueAccounts {
		key := account.TenantID*1000000000 + account.ID
		if _, ok := seenAccounts[key]; ok {
			continue
		}
		accounts = append(accounts, account)
		seenAccounts[key] = struct{}{}
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	enqueued := 0
	for _, account := range accounts {
		engine := s.accountPushEngine(ctx, account.TenantID, account.ID)
		if engine != pushEngineUnifiedStream {
			if account.TenantID == 78 && account.ID == 615 {
				g.Log().Info(ctx, "账户循环调度跳过", g.Map{"tenantId": account.TenantID, "accountId": account.ID, "pushEngine": engine, "reason": "push_engine_not_unified_stream"})
			}
			continue
		}
		body, err := json.Marshal(publishAccountScope{TenantID: account.TenantID, AccountID: account.ID})
		if err != nil {
			return err
		}
		// 账户调度使用独立队列，避免历史采集/资料任务占满后台队列后把
		// 循环计划延迟到下一轮；同一账户仍由租约保证串行，账户之间可并行。
		_, err = client.EnqueueContext(ctx, asynq.NewTask(publishconsts.QueueAccountScheduleTask, body), asynq.Queue(tgQueueNameAccountSchedule),
			asynq.Unique(2*time.Second), asynq.Timeout(25*time.Second), asynq.MaxRetry(3))
		if err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			g.Log().Warning(ctx, "账户循环调度入队失败", g.Map{"tenantId": account.TenantID, "accountId": account.ID, "queueName": tgQueueNameAccountSchedule, "error": err.Error()})
			return err
		}
		if account.TenantID == 78 && account.ID == 615 {
			g.Log().Info(ctx, "账户循环调度入队结果", g.Map{"tenantId": account.TenantID, "accountId": account.ID, "queueName": tgQueueNameAccountSchedule, "duplicate": errors.Is(err, asynq.ErrDuplicateTask), "error": ""})
		}
		enqueued++
	}
	if len(dueAccounts) > 0 || enqueued > 0 {
		g.Log().Info(ctx, "账户调度扫描完成", g.Map{"tenantAccounts": len(accounts), "enqueued": enqueued, "cursor": cursor.Int64(), "dueAccounts": len(dueAccounts)})
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
	g.Log().Info(ctx, "账户调度任务开始", g.Map{"tenantId": scope.TenantID, "accountId": scope.AccountID, "queueName": tgQueueNameAccountSchedule})
	startedAt := time.Now()
	defer func() {
		g.Log().Info(ctx, "账户调度任务结束", g.Map{"tenantId": scope.TenantID, "accountId": scope.AccountID, "queueName": tgQueueNameAccountSchedule, "elapsedMs": time.Since(startedAt).Milliseconds()})
	}()
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
		// 先清理已发送任务遗留的 Outbox，避免历史状态阻塞队首。
		func(ctx context.Context) error { return reconcileTerminalTelegramOutboxes(ctx, scope, 200) },
		// 先收口已经完成的资料级频道投影，避免历史投影一直停在 pending
		// 阻塞本账户下一轮循环。该阶段按账户、按小批次执行，不扫描其他账户。
		func(ctx context.Context) error { return reconcileProfileCycleRunChannels(ctx, scope, 200) },
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
		stageCtx, cancel := context.WithTimeout(bounded, accountScheduleStageTimeout())
		err := stage(stageCtx)
		budgetEnded := stageCtx.Err() != nil && bounded.Err() == nil
		cancel()
		if err != nil && budgetEnded {
			g.Log().Info(ctx, "账户调度达到本轮预算，剩余持久计划下轮继续", g.Map{"tenantId": scope.TenantID, "accountId": scope.AccountID, "stage": stageIndex + 1})
			continue
		}
		if err != nil {
			g.Log().Warningf(ctx, "账户调度阶段失败 tenantId:%d accountId:%d stage:%d err:%+v", scope.TenantID, scope.AccountID, stageIndex+1, err)
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// accountScheduleStageTimeout 为账户循环调度阶段提供独立预算，避免大账户每轮只能生成极少任务。
// 仍受账户总租约限制，不会恢复全局扫描；默认10秒，可通过环境变量调整。
func accountScheduleStageTimeout() time.Duration {
	minutes, err := strconv.Atoi(os.Getenv("YOUBAN_PUBLISH_ACCOUNT_SCHEDULE_STAGE_SECONDS"))
	if err != nil || minutes <= 0 {
		return 10 * time.Second
	}
	if minutes > 60 {
		minutes = 60
	}
	return time.Duration(minutes) * time.Second
}

// reconcileProfileCycleRunChannels 将已经有终态 Job 的资料级频道投影收口。
//
// 旧版本在 Job 已经 sent/superseded 后，可能因为回调中断或迁移任务提前结束，
// 没有同步更新 profile_cycle_run_channel，留下 pending 投影。调度器把这种投影
// 当作仍在执行，后续轮次便永远不会创建。这里仅处理当前账户且每次限制数量，
// 不重试 Telegram，也不修改仍在发送的 Job，保证幂等和频道隔离。
func reconcileProfileCycleRunChannels(ctx context.Context, scope publishAccountScope, limit int) error {
	if scope.TenantID <= 0 || scope.AccountID <= 0 {
		return nil
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := g.DB().GetAll(ctx, `WITH candidates AS (
		SELECT rc.id, rc.run_id,
			CASE
				WHEN j.status='sent' THEN 'sent'
				WHEN j.status='superseded' THEN 'superseded'
				WHEN j.status='failed' THEN 'failed'
				ELSE 'skipped'
			END AS projection_status,
			CASE
				WHEN j.status='superseded' THEN COALESCE(NULLIF(j.error_message, ''), '原任务已被替代')
				WHEN j.status='failed' THEN COALESCE(NULLIF(j.error_message, ''), 'Telegram任务已终态失败')
				WHEN j.id IS NULL THEN '频道已关闭或任务已清理'
				ELSE ''
			END AS error_message
		FROM `+publishProfileCycleRunChannelTable+` rc
		JOIN `+publishProfileCycleRunTable+` r ON r.id=rc.run_id
		LEFT JOIN `+publishTgJobTable+` j ON j.id=rc.job_id
		LEFT JOIN `+publishChannelTable+` c ON c.id=rc.channel_id AND c.deleted_at IS NULL
		WHERE rc.tenant_id=? AND rc.account_id=?
		  AND r.tenant_id=? AND r.account_id=?
		  AND rc.status NOT IN ('sent','superseded','failed','skipped')
		  AND (
				(j.status IN ('sent','superseded','failed') AND COALESCE(j.dispatch_status,'')='done')
				OR (rc.job_id=0 AND (c.id IS NULL OR c.status<>1 OR c.cycle_publish_enabled<>1 OR c.publish_direction<>'up'))
		  )
		ORDER BY rc.id
		LIMIT ?
	)
	UPDATE `+publishProfileCycleRunChannelTable+` rc
	SET status=c.projection_status,
		error_message=c.error_message,
		updated_at=NOW()
	FROM candidates c
	WHERE rc.id=c.id
	RETURNING rc.run_id`, scope.TenantID, scope.AccountID, scope.TenantID, scope.AccountID, limit)
	if err != nil {
		return gerror.Wrap(err, "收口资料循环频道投影失败")
	}
	if len(rows) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		runID := row["run_id"].Int64()
		if runID <= 0 {
			continue
		}
		if _, ok := seen[runID]; ok {
			continue
		}
		seen[runID] = struct{}{}
		if err := finishProfileCycleRun(ctx, runID); err != nil {
			return gerror.Wrapf(err, "收口资料循环运行状态失败 runId:%d", runID)
		}
	}
	g.Log().Info(ctx, "资料循环频道投影已收口", g.Map{
		"tenantId": scope.TenantID, "accountId": scope.AccountID, "count": len(rows), "runCount": len(seen),
	})
	return nil
}

// reconcileTerminalTelegramOutboxes 收口已经有终态 Job 的遗留 Outbox。
// 发送结果已经落库后，旧版本可能在 Outbox 回调前退出，留下 pending/processing；
// 这些记录不应再次参与调度，也不能阻塞同频道后续任务。按账户小批量处理，
// 只更新终态 Job，避免全局扫描和误触发 Telegram 重发。
func reconcileTerminalTelegramOutboxes(ctx context.Context, scope publishAccountScope, limit int) error {
	if scope.TenantID <= 0 || scope.AccountID <= 0 {
		return nil
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	_, err := g.DB().Exec(ctx, `WITH candidates AS (
		SELECT o.id
		FROM `+publishTgOutboxTable+` o
		JOIN `+publishTgJobTable+` j ON j.id=o.job_id
		WHERE j.tenant_id=? AND j.account_id=?
		  AND j.status IN ('sent','superseded','failed')
		  AND o.status IN ('pending','processing')
		ORDER BY o.id
		LIMIT ?
	)
	UPDATE `+publishTgOutboxTable+` o
	SET status='delivered', lease_owner='', lease_expires_at=NULL,
	    delivered_at=COALESCE(o.delivered_at,NOW()), last_error='', updated_at=NOW()
	FROM candidates c
	WHERE o.id=c.id`, scope.TenantID, scope.AccountID, limit)
	if err != nil {
		return gerror.Wrap(err, "收口终态TG任务Outbox失败")
	}
	return nil
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
