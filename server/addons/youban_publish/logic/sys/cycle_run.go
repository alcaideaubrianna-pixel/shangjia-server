package sys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
)

const (
	cycleRunStatusPending     = "pending"
	cycleRunStatusRunning     = "running"
	cycleRunStatusDispatching = "dispatching"
	cycleRunStatusFinished    = "finished"
	cycleRunStatusPartial     = "partial_failed"
	cycleRunStatusFailed      = "failed"
	cycleRunStatusSkipped     = "skipped"
	cycleBatchPageSize        = 200
	cycleBatchBacklogLimit    = 1000
)

type channelCycleRecord struct {
	Id               int64  `orm:"id"`
	TenantId         int64  `orm:"tenant_id"`
	Enabled          int    `orm:"cycle_publish_enabled"`
	Status           int    `orm:"status"`
	PublishDirection string `orm:"publish_direction"`
	Mode             string `orm:"cycle_publish_mode"`
	BatchSize        int    `orm:"cycle_batch_size"`
	BatchCursor      int64  `orm:"cycle_batch_cursor"`
}

type cycleRunRecord struct {
	Id             int64       `orm:"id"`
	TenantId       int64       `orm:"tenant_id"`
	ChannelId      int64       `orm:"channel_id"`
	Status         string      `orm:"status"`
	Stage          string      `orm:"stage"`
	CursorId       int64       `orm:"cursor_id"`
	UpperProfileID int64       `orm:"upper_profile_id"`
	TotalCount     int         `orm:"total_count"`
	QueuedCount    int         `orm:"queued_count"`
	ScheduledAt    *gtime.Time `orm:"scheduled_at"`
	StartedAt      *gtime.Time `orm:"started_at"`
	FinishedAt     *gtime.Time `orm:"finished_at"`
	ErrorMessage   string      `orm:"error_message"`
}

type channelProfileRecord struct {
	Id        int64 `orm:"id"`
	TenantId  int64 `orm:"tenant_id"`
	AccountId int64 `orm:"account_id"`
	ChannelId int64 `orm:"channel_id"`
	ProfileId int64 `orm:"profile_id"`
}

func (s *sSysPublish) RunChannelCycleScheduler(ctx context.Context) error {
	if globalStreamPushEngineEnabled() {
		return s.enqueueAccountSchedules(ctx)
	}
	if err := s.enqueueAccountSchedules(ctx); err != nil {
		return err
	}
	return runChannelCycleSchedulerStages(ctx, []channelCycleSchedulerStage{
		// 到期循环是用户可见的主路径，必须先进入分片扫描；恢复和通知阶段
		// 可能访问大量历史数据，不能阻塞新一轮循环调度。
		{name: "扫描批次循环", run: func(ctx context.Context) error { return s.scheduleDueChannelBatchCycles(ctx, 50) }},
		{name: "扫描时间循环", run: s.runProfileCycleDueScan},
		{name: "暂停长期未活跃免费循环", run: func(ctx context.Context) error { return s.pauseInactiveFreeChannelCycles(ctx) }},
		{name: "恢复循环批次", run: func(ctx context.Context) error { return s.recoverChannelCycleRuns(ctx) }},
		{name: "收口下架循环任务", run: func(ctx context.Context) error { return s.supersedeOfflineCycleJobs(ctx, 200) }},
		{name: "核对循环补偿", run: func(ctx context.Context) error { return s.reconcileRecoveredChannelCycleRuns(ctx, 20) }},
		{name: "收尾循环批次", run: func(ctx context.Context) error { return s.finalizeDispatchingChannelCycleRuns(ctx, 20) }},
		{name: "恢复到期时间循环", run: func(ctx context.Context) error { return s.enqueueDueProfileCycleReschedules(ctx, 20) }},
		{name: "恢复循环重算", run: func(ctx context.Context) error { return s.enqueuePendingProfileCycleReschedules(ctx, 200) }},
	})
}

// supersedeOfflineCycleJobs 收口历史上已经生成但资料后来下架的循环任务。
// 只处理未发送任务，保留已发送历史审计，避免下架资料继续占用频道队首。
func (s *sSysPublish) supersedeOfflineCycleJobs(ctx context.Context, limit int) error {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	headTimeoutMinutes := cycleHeadTimeoutMinutes()
	_, err := g.DB().Exec(ctx, `WITH candidates AS (
		SELECT j.id,p.status AS profile_status,p.visibility AS profile_visibility,p.deleted_at AS profile_deleted_at FROM `+publishTgJobTable+` j
		JOIN hg_content_profile p ON p.id=j.profile_id
		WHERE (j.operation_no LIKE 'cycle_%' OR j.operation_no LIKE 'profile_cycle:%')
		  AND j.status IN ('pending','failed_retry','unknown','sending','confirming')
		  AND (p.status<>1 OR p.visibility<>? OR p.deleted_at IS NOT NULL
		       OR j.created_at < NOW() - (? * INTERVAL '1 minute'))
		ORDER BY j.id LIMIT `+strconv.Itoa(limit)+`
	)
	UPDATE `+publishTgJobTable+` j SET status='superseded',dispatch_status=?,send_phase=CASE WHEN j.created_at < NOW() - (? * INTERVAL '1 minute') AND c.profile_status=1 AND c.profile_visibility=? AND c.profile_deleted_at IS NULL THEN 'skipped_stale_cycle' ELSE 'skipped_offline_profile' END,next_retry_at=NULL,last_dispatch_error=CASE WHEN j.created_at < NOW() - (? * INTERVAL '1 minute') AND c.profile_status=1 AND c.profile_visibility=? AND c.profile_deleted_at IS NULL THEN '循环任务等待超过15分钟，已跳过以释放频道队首' ELSE '资料已下架或不可公开，跳过循环翻新' END,updated_at=NOW()
	FROM candidates c
	WHERE j.id=c.id`, consts.ContentVisibilityPublic, headTimeoutMinutes, tgDispatchStatusDone, headTimeoutMinutes, consts.ContentVisibilityPublic, headTimeoutMinutes, consts.ContentVisibilityPublic)
	if err != nil {
		return gerror.Wrap(err, "收口下架循环任务失败")
	}
	if _, err = g.DB().Exec(ctx, `UPDATE `+publishTgOutboxTable+` o SET status='delivered',lease_owner='',lease_expires_at=NULL,delivered_at=COALESCE(delivered_at,NOW()),last_error='循环任务已自动收口',updated_at=NOW() FROM `+publishTgJobTable+` j WHERE j.id=o.job_id AND j.send_phase IN ('skipped_stale_cycle','skipped_offline_profile') AND o.status IN ('pending','processing')`); err != nil {
		return gerror.Wrap(err, "收口循环任务Outbox失败")
	}
	return nil
}

// cycleHeadTimeoutMinutes 返回循环队首最大等待时间，默认15分钟，可通过环境变量调整。
func cycleHeadTimeoutMinutes() int {
	value, err := strconv.Atoi(os.Getenv("YOUBAN_PUBLISH_CYCLE_HEAD_TIMEOUT_MINUTES"))
	if err != nil || value <= 0 {
		return 15
	}
	if value > 24*60 {
		return 24 * 60
	}
	return value
}

// pauseInactiveFreeChannelCycles 在指定账户范围复用免费频道长期不活跃保护规则。
func (s *sSysPublish) pauseInactiveFreeChannelCycles(ctx context.Context, scopes ...publishAccountScope) error {
	inactiveDays := maxConfigInt(ctx, "youbanPublish.cycle.freeInactiveDays", 7)
	cutoff := gtime.Now().Add(-time.Duration(inactiveDays) * 24 * time.Hour)
	var channels []*channelCycleRecord
	model := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Fields("id,tenant_id,cycle_publish_enabled,status,publish_direction,cycle_publish_mode").Where("cycle_publish_enabled", 1).Where("status", 1).Where("publish_direction", "up")
	model = accountScheduleModel(model, publishChannelTable+".tenant_id", publishChannelTable+".tg_account_id", scopes)
	if err := model.Scan(&channels); err != nil {
		return gerror.Wrap(err, "读取免费循环频道失败")
	}
	notified := make(map[int64]bool)
	inactiveByTenant := make(map[int64]bool)
	for _, channel := range channels {
		inactive, loaded := inactiveByTenant[channel.TenantId]
		if !loaded {
			vip, err := s.tenantVipStatus(ctx, channel.TenantId)
			if err != nil {
				return err
			}
			if !tenantVipStatusActive(vip) {
				var activity struct {
					LastActiveAt *gtime.Time `orm:"last_active_at"`
				}
				err := pdao.YoubanPublishAccount.Ctx(ctx).Fields("last_active_at").Where("tenant_id", channel.TenantId).Where("account_type", sysin.PublishAccountTypeAdmin).Where("status", 1).OrderDesc("last_active_at").Limit(1).Scan(&activity)
				if errors.Is(err, sql.ErrNoRows) && len(scopes) > 0 {
					// 历史账户可能只有上传账户：复用本账户活动时间，缺少管理员不能导致保护永久失效。
					err = pdao.YoubanPublishAccount.Ctx(ctx).Fields("last_active_at").Where("tenant_id", channel.TenantId).Where("id", scopes[0].AccountID).Where("status", 1).Scan(&activity)
				}
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}
				inactive = activity.LastActiveAt == nil || activity.LastActiveAt.Before(cutoff)
			}
			inactiveByTenant[channel.TenantId] = inactive
		}
		if !inactive {
			continue
		}
		if _, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).Where("cycle_publish_enabled", 1).Data(g.Map{"cycle_publish_enabled": 0, "updated_at": gtime.Now()}).Update(); err != nil {
			return err
		}
		_ = s.syncChannelCycleAfterSave(ctx, channel.TenantId, channel.Id, 0, 0, "")
		if notified[channel.TenantId] {
			continue
		}
		notified[channel.TenantId] = true
		if accountId, notifyErr := s.tenantVipNotifyAccountId(ctx, channel.TenantId, 0); notifyErr == nil && accountId > 0 {
			_ = s.notifyAccountWithPreference(ctx, channel.TenantId, "channel", &botsysin.NotifyAccountInp{BotStrategy: "official", FallbackBoundBot: true, IgnoreFeatureSwitch: true, App: consts.AppApi, AccountId: accountId, Text: fmt.Sprintf("为避免长期未使用的频道循环推送持续占用服务器资源，保障整体服务稳定运行，因连续 %d 天未使用后台，频道循环推送已暂时关闭。\n\n重新进入后台后，可在频道配置中手动恢复。", inactiveDays)})
		}
	}
	return nil
}

func (s *sSysPublish) reconcileRecoveredChannelCycleRuns(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	var runs []cycleRunRecord
	if err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,channel_id,status").
		Where("status", cycleRunStatusPartial).
		OrderAsc("updated_at").Limit(limit).Scan(&runs); err != nil {
		return gerror.Wrap(err, "读取待核对循环补偿批次失败")
	}
	for _, run := range runs {
		prefix := fmt.Sprintf("cycle_batch:%d:%%", run.Id)
		message := "原循环任务已由自动补偿任务成功替代"
		_, err := g.DB().Exec(ctx, `UPDATE `+publishTgJobTable+` source
			SET status='superseded', dispatch_status=?, next_retry_at=NULL,
				last_dispatch_error=?, updated_at=NOW()
			WHERE source.status='failed' AND source.operation_no LIKE ?
				AND EXISTS (
					SELECT 1 FROM `+publishTgJobTable+` recovery
					WHERE recovery.operation_no LIKE ('auto-recover:' || source.id || ':%')
						AND recovery.profile_id=source.profile_id
						AND recovery.channel_id=source.channel_id
						AND recovery.status='sent'
				)`, tgDispatchStatusDone, message, prefix)
		if err != nil {
			return gerror.Wrap(err, "核对循环批次自动补偿任务失败")
		}
		if err = s.reconcileCycleRunStatus(ctx, fmt.Sprintf("cycle_batch:%d:", run.Id)); err != nil {
			return err
		}
	}
	return nil
}

type channelCycleSchedulerStage struct {
	name string
	run  func(context.Context) error
}

func runChannelCycleSchedulerStages(ctx context.Context, stages []channelCycleSchedulerStage) error {
	errs := make([]error, 0, len(stages))
	for _, stage := range stages {
		if stage.run == nil {
			continue
		}
		startedAt := time.Now()
		g.Log().Infof(ctx, "频道循环调度阶段开始 stage:%s", stage.name)
		if err := stage.run(ctx); err != nil {
			g.Log().Warningf(ctx, "频道循环调度阶段失败 stage:%s elapsed_ms:%d err:%+v", stage.name, time.Since(startedAt).Milliseconds(), err)
			errs = append(errs, fmt.Errorf("%s: %w", stage.name, err))
			continue
		}
		g.Log().Infof(ctx, "频道循环调度阶段完成 stage:%s elapsed_ms:%d", stage.name, time.Since(startedAt).Milliseconds())
	}
	return errors.Join(errs...)
}

func (s *sSysPublish) AdminChannelCycleRun(ctx context.Context, in *sysin.ChannelCycleRunInp) (*sysin.ChannelFullPushModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.ChannelId <= 0 {
		return nil, gerror.New("请选择频道")
	}
	channel, err := s.fullPushChannel(ctx, account.TenantId, in.ChannelId)
	if err != nil {
		return nil, err
	}
	var runId int64
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		locked, lockErr := tx.Model(publishChannelTable).Safe().Ctx(ctx).
			Where("id", channel.Id).Where("tenant_id", account.TenantId).
			Where("cycle_active_run_id", 0).WhereNull("deleted_at").
			Data(g.Map{"cycle_active_run_id": -1, "updated_at": now}).Update()
		if lockErr != nil {
			return gerror.Wrap(lockErr, "锁定频道循环配置失败")
		}
		affected, _ := locked.RowsAffected()
		if affected == 0 {
			return gerror.New("该频道已有循环曝光正在执行")
		}
		count, countErr := s.fullPushEligibleProfileModel(ctx, account.TenantId, channel.Id).Count()
		if countErr != nil {
			return gerror.Wrap(countErr, "统计频道循环资料失败")
		}
		runId, err = tx.Model(publishCycleRunTable).Ctx(ctx).Data(g.Map{
			"plan_id": 0, "tenant_id": account.TenantId, "account_id": 0, "profile_id": 0,
			"channel_id": channel.Id, "status": cycleRunStatusPending, "stage": "manual",
			"cursor_id": 0, "total_count": count, "queued_count": 0,
			"scheduled_at": nil, "created_at": now, "updated_at": now,
		}).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "创建手动循环批次失败")
		}
		_, err = tx.Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).
			Data(g.Map{"cycle_active_run_id": runId, "updated_at": now}).Update()
		return err
	})
	if err != nil {
		return nil, err
	}
	if err = s.enqueueCycleRun(ctx, runId, 0); err != nil {
		s.failChannelCycleRun(ctx, runId, channel.Id, err)
		return nil, gerror.Wrap(err, "手动循环批次入队失败")
	}
	g.Log().Info(ctx, "频道循环上架已触发", g.Map{
		"tenantId": account.TenantId, "channelId": channel.Id, "runId": runId,
		"status": cycleRunStatusPending,
	})
	return &sysin.ChannelFullPushModel{ChannelId: channel.Id, Queued: 0, BatchNo: fmt.Sprintf("cycle_batch:%d", runId), Status: cycleRunStatusPending}, nil
}

// recoverChannelCycleRuns 恢复账户范围的滞留批次，新生产任务采用与短执行预算对应的恢复阈值。
func (s *sSysPublish) recoverChannelCycleRuns(ctx context.Context, scopes ...publishAccountScope) error {
	now := gtime.Now()
	staleRunningBefore := now.Add(-35 * time.Minute)
	staleDispatchingBefore := now.Add(-30 * time.Minute)
	if len(scopes) > 0 {
		staleRunningBefore = now.Add(-35 * time.Second)
	}
	var staleRunning []cycleRunRecord
	if err := cycleRunAccountModel(ctx, scopes).
		Fields("id,channel_id").
		Where("status", cycleRunStatusRunning).
		WhereLTE("updated_at", staleRunningBefore).
		OrderAsc("id").
		Limit(20).
		Scan(&staleRunning); err != nil {
		return gerror.Wrap(err, "读取超时频道循环批次失败")
	}
	for _, run := range staleRunning {
		result, err := cycleRunAccountModel(ctx, scopes).
			Where("id", run.Id).
			Where("status", cycleRunStatusRunning).
			WhereLTE("updated_at", staleRunningBefore).
			Data(g.Map{
				"status":        cycleRunStatusFailed,
				"stage":         "recovering",
				"error_message": "循环批次执行超时，已由定时调度恢复",
				"updated_at":    now,
			}).
			Update()
		if err != nil {
			return gerror.Wrap(err, "恢复超时频道循环批次失败")
		}
		affected, _ := result.RowsAffected()
		if affected > 0 {
			if err = s.enqueueCycleRun(ctx, run.Id, 0); err != nil {
				return gerror.Wrap(err, "恢复超时频道循环批次入队失败")
			}
		}
	}

	// A dispatching run is normally finalized by terminal-state polling. If it
	// has been idle for too long and has no unfinished child jobs, release it
	// as a partial failure. Never re-enqueue a run with pending/sending jobs,
	// otherwise Telegram messages could be sent twice.
	var staleDispatching []cycleRunRecord
	if err := cycleRunAccountModel(ctx, scopes).
		Fields("id,channel_id").
		Where("status", cycleRunStatusDispatching).
		WhereLTE("updated_at", staleDispatchingBefore).
		OrderAsc("id").Limit(20).Scan(&staleDispatching); err != nil {
		return gerror.Wrap(err, "读取超时频道循环发送批次失败")
	}
	for _, run := range staleDispatching {
		active, err := s.cycleRunHasUnfinishedJobs(ctx, run.Id)
		if err != nil {
			return err
		}
		if active {
			continue
		}
		if err = s.finalizeChannelCycleDelivery(ctx, run); err != nil {
			return err
		}
		var current cycleRunRecord
		if err = cycleRunAccountModel(ctx, scopes).
			Fields("id,status").Where("id", run.Id).Scan(&current); err != nil {
			return gerror.Wrap(err, "校验超时频道循环发送状态失败")
		}
		if current.Status != cycleRunStatusDispatching {
			continue
		}
		if err = s.markStaleDispatchingCycleRun(ctx, run, now); err != nil {
			return err
		}
	}

	var waiting []cycleRunRecord
	if err := cycleRunAccountModel(ctx, scopes).
		Fields("id,channel_id").
		WhereIn("status", []string{cycleRunStatusPending, cycleRunStatusFailed}).
		WhereLTE("updated_at", now.Add(-90*time.Second)).
		OrderAsc("id").
		Limit(50).
		Scan(&waiting); err != nil {
		return gerror.Wrap(err, "读取滞留频道循环批次失败")
	}
	for _, run := range waiting {
		if err := s.enqueueCycleRun(ctx, run.Id, 0); err != nil {
			return gerror.Wrap(err, "恢复滞留频道循环批次入队失败")
		}
	}
	return nil
}

func (s *sSysPublish) cycleRunHasUnfinishedJobs(ctx context.Context, runId int64) (bool, error) {
	count, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereLike("operation_no", fmt.Sprintf("cycle_batch:%d:", runId)+"%").
		WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown"}).Count()
	return count > 0, err
}

func (s *sSysPublish) markStaleDispatchingCycleRun(ctx context.Context, run cycleRunRecord, now *gtime.Time) error {
	message := "循环批次发送状态超时且无未完成任务，已释放并等待下次调度"
	result, err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).
		Where("id", run.Id).Where("status", cycleRunStatusDispatching).
		Data(g.Map{"status": cycleRunStatusPartial, "stage": "finished", "error_message": message, "finished_at": now, "updated_at": now}).Update()
	if err != nil {
		return gerror.Wrap(err, "释放超时频道循环发送批次失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	_, _ = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("id", run.ChannelId).Where("cycle_active_run_id", run.Id).
		Data(g.Map{"cycle_active_run_id": 0, "cycle_last_run_at": now, "cycle_last_error_message": message, "updated_at": now}).Update()
	s.appendChannelCycleRunLog(ctx, run, "warning", "stale_dispatching_recovered", message, nil)
	return nil
}

func (s *sSysPublish) ExecuteCycleRun(ctx context.Context, runId int64) error {
	run, err := s.lockChannelCycleRun(ctx, runId)
	if err != nil || run.Id <= 0 {
		return err
	}
	channel, err := s.channelCycleById(ctx, run.ChannelId)
	if err != nil {
		s.failChannelCycleRun(ctx, run.Id, run.ChannelId, err)
		return err
	}
	if channel.Id <= 0 || channel.Enabled != 1 || channel.Status != 1 || channel.PublishDirection != "up" {
		s.finishChannelCycleRun(ctx, run, cycleRunStatusSkipped, "频道循环配置已关闭")
		return nil
	}
	if scheduledBatchCycleRun(run) && channel.Mode != "batch" {
		s.finishChannelCycleRun(ctx, run, cycleRunStatusSkipped, "批次循环已切换为时间循环")
		return nil
	}
	backlog, err := s.channelCycleBacklog(ctx, run.ChannelId)
	if err != nil {
		s.failChannelCycleRun(ctx, run.Id, run.ChannelId, err)
		return err
	}
	if backlog >= cycleBatchBacklogLimit {
		return s.continueChannelCycleRun(ctx, run, 30*time.Second)
	}
	pageLimit := cycleBatchPageSize
	if channel.Mode == "batch" {
		remaining := batchCycleRunRemaining(run)
		if remaining <= 0 {
			return s.beginChannelCycleDispatch(ctx, run)
		}
		if remaining < pageLimit {
			pageLimit = remaining
		}
	}
	items, err := s.channelCyclePage(ctx, channel, run.CursorId, pageLimit, run.UpperProfileID)
	if err != nil {
		s.failChannelCycleRun(ctx, run.Id, run.ChannelId, err)
		return err
	}
	if len(items) == 0 {
		return s.beginChannelCycleDispatch(ctx, run)
	}
	lastCursor := run.CursorId
	queued := 0
	for _, item := range items {
		created, enqueueErr := s.enqueueChannelCycleProfile(ctx, run.Id, item)
		if enqueueErr != nil {
			if errors.Is(enqueueErr, errTelegramPublishPermissionPaused) {
				if updateErr := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
					return finishPermissionPausedCycleRunTx(ctx, tx, run, lastCursor, queued)
				}); updateErr != nil {
					return updateErr
				}
				s.appendChannelCycleRunLog(ctx, run, "info", "permission_paused", enqueueErr.Error(), nil)
				return nil
			}
			err = enqueueErr
			s.failChannelCycleRun(ctx, run.Id, run.ChannelId, err)
			return err
		}
		if item.Id > lastCursor {
			lastCursor = item.Id
		}
		if created {
			queued++
		}
	}
	result, err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", run.Id).Where("status", cycleRunStatusRunning).Data(g.Map{
		"cursor_id": lastCursor, "queued_count": run.QueuedCount + queued,
		"status": cycleRunStatusPending, "stage": "producing", "error_message": "", "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新频道循环批次游标失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	return s.enqueueCycleRun(ctx, run.Id, time.Second)
}

// The run snapshot is the hard upper bound for one scheduled batch. The
// channel batch size is only used when the run is created; using it again here
// makes a short candidate set wrap and count the same profiles more than once.
func batchCycleRunRemaining(run cycleRunRecord) int {
	remaining := run.TotalCount - run.QueuedCount
	if remaining < 0 {
		return 0
	}
	return remaining
}

func finishPermissionPausedCycleRunTx(ctx context.Context, tx gdb.TX, run cycleRunRecord, cursor int64, queued int) error {
	result, err := tx.Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", run.Id).Where("status", cycleRunStatusRunning).
		Data(g.Map{"status": cycleRunStatusSkipped, "stage": "finished", "cursor_id": cursor, "queued_count": run.QueuedCount + queued,
			"error_message": errTelegramPublishPermissionPaused.Error(), "finished_at": gtime.Now()}).Update()
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return err
	}
	_, err = tx.Model(publishChannelTable).Safe().Ctx(ctx).Where("id", run.ChannelId).Where("cycle_active_run_id", run.Id).
		Data(g.Map{"cycle_active_run_id": 0, "cycle_last_error_message": errTelegramPublishPermissionPaused.Error(), "cycle_last_run_at": gtime.Now()}).Update()
	return err
}

func scheduledBatchCycleRun(run cycleRunRecord) bool {
	return run.ScheduledAt != nil
}

func (s *sSysPublish) beginChannelCycleDispatch(ctx context.Context, run cycleRunRecord) error {
	result, err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", run.Id).Where("status", cycleRunStatusRunning).Data(g.Map{
		"status": cycleRunStatusDispatching, "stage": "dispatching", "error_message": "", "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新频道循环等待发送状态失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	run.Status = cycleRunStatusDispatching
	return s.finalizeChannelCycleDelivery(ctx, run)
}

// finalizeDispatchingChannelCycleRuns 按账户汇总已投递批次的频道结果并释放下一轮入口。
func (s *sSysPublish) finalizeDispatchingChannelCycleRuns(ctx context.Context, limit int, scopes ...publishAccountScope) error {
	var runs []cycleRunRecord
	if err := cycleRunAccountModel(ctx, scopes).
		Where("status", cycleRunStatusDispatching).
		OrderAsc("id").Limit(limit).Scan(&runs); err != nil {
		return gerror.Wrap(err, "读取等待发送完成的循环批次失败")
	}
	for _, run := range runs {
		if err := s.finalizeChannelCycleDelivery(ctx, run); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) finalizeChannelCycleDelivery(ctx context.Context, run cycleRunRecord) error {
	done, status, message, err := publishBatchTerminalState(ctx, fmt.Sprintf("cycle_batch:%d:", run.Id))
	if err != nil || !done {
		return err
	}
	cycleStatus := cycleRunStatusFinished
	if status == "failed" {
		cycleStatus = cycleRunStatusFailed
	} else if status == "partial_failed" {
		cycleStatus = cycleRunStatusPartial
	}
	s.finishChannelCycleRun(ctx, run, cycleStatus, message)
	return nil
}

func (s *sSysPublish) lockChannelCycleRun(ctx context.Context, runId int64) (cycleRunRecord, error) {
	var run cycleRunRecord
	result, err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).
		Where("id", runId).
		WhereIn("status", []string{cycleRunStatusPending, cycleRunStatusFailed}).
		Data(g.Map{"status": cycleRunStatusRunning, "stage": "producing", "started_at": gtime.Now(), "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return run, gerror.Wrap(err, "锁定频道循环批次失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return run, nil
	}
	if err = g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", runId).Scan(&run); err != nil {
		return run, gerror.Wrap(err, "读取频道循环批次失败")
	}
	return run, nil
}

func (s *sSysPublish) channelCycleById(ctx context.Context, channelId int64) (channelCycleRecord, error) {
	var channel channelCycleRecord
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,cycle_publish_enabled,status,publish_direction,cycle_publish_mode,cycle_batch_size,cycle_batch_cursor").
		Where("id", channelId).
		WhereNull("deleted_at").
		Scan(&channel)
	return channel, err
}

// channelCyclePage 使用资料主键和本轮上界分页，新增资料不会延长正在执行的轮次。
func (s *sSysPublish) channelCyclePage(ctx context.Context, channel channelCycleRecord, cursorId int64, limit int, upperIDs ...int64) ([]channelProfileRecord, error) {
	var items []channelProfileRecord
	model := s.fullPushEligibleProfileModel(ctx, channel.TenantId, channel.Id)
	if channel.Mode == "batch" {
		model = s.batchCycleEligibleProfileModel(ctx, channel.TenantId, channel.Id)
	}
	query := model.Fields("p.id AS id,ps.tenant_id,ps.account_id,p.id AS profile_id")
	// A persisted profile ID is monotonic and cannot represent a mid-run wrap.
	// Finish the tail in this run; the scheduler resets to zero only when the
	// next run observes that no IDs remain after the cursor.
	query = query.WhereGT("p.id", cursorId).OrderAsc("p.id")
	if len(upperIDs) > 0 && upperIDs[0] > 0 {
		query = query.WhereLTE("p.id", upperIDs[0])
	}
	err := query.Limit(limit).Scan(&items)
	if err != nil {
		return nil, gerror.Wrap(err, "分页读取频道循环资料失败")
	}
	for i := range items {
		items[i].ChannelId = channel.Id
	}
	return items, nil
}

func (s *sSysPublish) channelCycleBacklog(ctx context.Context, channelId int64) (int, error) {
	return g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("channel_id", channelId).
		WhereLike("operation_no", "cycle_batch:%").
		WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown"}).
		Count()
}

func (s *sSysPublish) enqueueChannelCycleProfile(ctx context.Context, runId int64, item channelProfileRecord) (bool, error) {
	operationNo := cyclePublishOperationNo(runId, item.ProfileId, item.ChannelId)
	err := s.submitProfilePublish(ctx, item.ProfileId, item.TenantId, item.AccountId, 0, operationNo, []int64{item.ChannelId}, true)
	if err != nil {
		if errors.Is(err, errPublishProfileUnavailable) {
			return false, s.deactivateChannelProfile(ctx, item.ChannelId, item.ProfileId)
		}
		return false, err
	}
	return true, nil
}

func cyclePublishOperationNo(runId int64, profileId int64, channelId int64) string {
	return fmt.Sprintf("cycle_batch:%d:%d:%d", runId, profileId, channelId)
}

func (s *sSysPublish) continueChannelCycleRun(ctx context.Context, run cycleRunRecord, delay time.Duration) error {
	result, err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", run.Id).Where("status", cycleRunStatusRunning).Data(g.Map{
		"status": cycleRunStatusPending, "stage": "backpressure", "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	return s.enqueueCycleRun(ctx, run.Id, delay)
}

func (s *sSysPublish) finishChannelCycleRun(ctx context.Context, run cycleRunRecord, status string, message string) {
	now := gtime.Now()
	_, _ = g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", run.Id).Data(g.Map{
		"status": status, "stage": "finished", "error_message": message, "finished_at": now, "updated_at": now,
	}).Update()
	data := g.Map{
		"cycle_active_run_id": 0, "cycle_last_run_at": now, "cycle_last_error_message": message, "updated_at": now,
	}
	if status == cycleRunStatusFinished || status == cycleRunStatusPartial {
		data["cycle_batch_cursor"] = run.CursorId
		data["cycle_batch_cursor_kind"] = "profile"
	}
	_, _ = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("id", run.ChannelId).Where("cycle_active_run_id", run.Id).Data(data).Update()
	s.appendChannelCycleRunLog(ctx, run, "info", "finished", message, nil)
}

func (s *sSysPublish) failChannelCycleRun(ctx context.Context, runId int64, channelId int64, cause error) {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	_, _ = g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", runId).Data(g.Map{
		"status": cycleRunStatusFailed, "stage": "failed", "error_message": message, "updated_at": gtime.Now(),
	}).Update()
	_, _ = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channelId).Data(g.Map{
		"cycle_last_error_message": message, "updated_at": gtime.Now(),
	}).Update()
}

func (s *sSysPublish) appendChannelCycleRunLog(ctx context.Context, run cycleRunRecord, level string, stage string, message string, contextMap g.Map) {
	var contextJSON any
	if len(contextMap) > 0 {
		if data, err := json.Marshal(contextMap); err == nil {
			contextJSON = string(data)
		}
	}
	_, _ = g.DB().Model(publishCycleRunLogTable).Safe().Ctx(ctx).Data(g.Map{
		"run_id": run.Id, "plan_id": 0, "tenant_id": run.TenantId, "account_id": 0,
		"profile_id": 0, "channel_id": run.ChannelId, "level": level, "stage": stage,
		"message": strings.TrimSpace(message), "context_json": contextJSON, "created_at": gtime.Now(),
	}).Insert()
}

func (s *sSysPublish) AIOpsChannelCycleRun(ctx context.Context, tenantId, channelId int64) (*sysin.ChannelFullPushModel, error) {
	if tenantId <= 0 || channelId <= 0 {
		return nil, gerror.New("租户或频道不能为空")
	}
	channel, err := s.fullPushChannel(ctx, tenantId, channelId)
	if err != nil {
		return nil, err
	}
	// Time-mode channels use per-profile due plans. AI compensation must rebuild
	// those plans from each profile's last successful send; creating a channel
	// batch here would incorrectly enqueue every active profile.
	if strings.EqualFold(strings.TrimSpace(channel.CyclePublishMode), "time") {
		if err := s.rescheduleChannelProfileCycles(ctx, channel.Id); err != nil {
			return nil, gerror.Wrap(err, "重算频道资料循环计划失败")
		}
		return &sysin.ChannelFullPushModel{ChannelId: channel.Id, Queued: 0, BatchNo: fmt.Sprintf("profile_cycle:repair:%d", channel.Id), Status: cycleRunStatusPending}, nil
	}
	var runId int64
	var totalCount int
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		locked, lockErr := tx.Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).Where("tenant_id", tenantId).Where("cycle_active_run_id", 0).WhereNull("deleted_at").Data(g.Map{"cycle_active_run_id": -1, "updated_at": now}).Update()
		if lockErr != nil {
			return gerror.Wrap(lockErr, "锁定频道循环配置失败")
		}
		affected, _ := locked.RowsAffected()
		if affected == 0 {
			return gerror.New("该频道已有循环曝光正在执行")
		}
		count, countErr := s.fullPushEligibleProfileModel(ctx, tenantId, channel.Id).Count()
		if countErr != nil {
			return gerror.Wrap(countErr, "统计频道循环资料失败")
		}
		totalCount = count
		runId, err = tx.Model(publishCycleRunTable).Ctx(ctx).Data(g.Map{"plan_id": 0, "tenant_id": tenantId, "account_id": 0, "profile_id": 0, "channel_id": channel.Id, "status": cycleRunStatusPending, "stage": "manual", "cursor_id": 0, "total_count": count, "queued_count": 0, "scheduled_at": nil, "created_at": now, "updated_at": now}).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "创建手动循环批次失败")
		}
		_, err = tx.Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channel.Id).Data(g.Map{"cycle_active_run_id": runId, "updated_at": now}).Update()
		return err
	})
	if err != nil {
		return nil, err
	}
	if err = s.enqueueCycleRun(ctx, runId, 0); err != nil {
		s.failChannelCycleRun(ctx, runId, channel.Id, err)
		return nil, gerror.Wrap(err, "手动循环批次入队失败")
	}
	g.Log().Info(ctx, "AI Ops 触发频道循环上架", g.Map{
		"tenantId": tenantId, "channelId": channel.Id, "runId": runId,
		"totalCount": totalCount, "status": cycleRunStatusPending,
	})
	return &sysin.ChannelFullPushModel{ChannelId: channel.Id, Queued: 0, BatchNo: fmt.Sprintf("cycle_batch:%d", runId), Status: cycleRunStatusPending}, nil
}
