package sys

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	cyclePlanStatusActive   = "active"
	cyclePlanStatusDisabled = "disabled"
	cycleRunStatusPending   = "pending"
	cycleRunStatusRunning   = "running"
	cycleRunStatusSuccess   = "success"
	cycleRunStatusFailed    = "failed"
	cycleRunStatusSkipped   = "skipped"
)

type cyclePlanRecord struct {
	Id              int64       `json:"id"`
	TenantId        int64       `json:"tenantId"`
	AccountId       int64       `json:"accountId"`
	ProfileId       int64       `json:"profileId"`
	TaskId          int64       `json:"taskId"`
	Enabled         int         `json:"enabled"`
	IntervalSeconds int         `json:"intervalSeconds"`
	PublishTime     string      `json:"publishTime"`
	NextRunAt       *gtime.Time `json:"nextRunAt"`
	Status          string      `json:"status"`
	LastRunId       int64       `json:"lastRunId"`
	LastRunAt       *gtime.Time `json:"lastRunAt"`
	LockedAt        *gtime.Time `json:"lockedAt"`
	CreatedAt       *gtime.Time `json:"createdAt"`
	UpdatedAt       *gtime.Time `json:"updatedAt"`
}

type cycleRunRecord struct {
	Id           int64       `json:"id"`
	PlanId       int64       `json:"planId"`
	TenantId     int64       `json:"tenantId"`
	AccountId    int64       `json:"accountId"`
	ProfileId    int64       `json:"profileId"`
	TaskId       int64       `json:"taskId"`
	Status       string      `json:"status"`
	Stage        string      `json:"stage"`
	ScheduledAt  *gtime.Time `json:"scheduledAt"`
	StartedAt    *gtime.Time `json:"startedAt"`
	FinishedAt   *gtime.Time `json:"finishedAt"`
	ErrorMessage string      `json:"errorMessage"`
	RetryCount   int         `json:"retryCount"`
}

func (s *sSysPublish) runCyclePlanScheduler(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	time.Sleep(3 * time.Second)
	if err := s.bootstrapCyclePlans(ctx); err != nil {
		g.Log().Warningf(ctx, "初始化循环上架计划失败：%+v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.scheduleDueCyclePlans(ctx, 100); err != nil {
				g.Log().Warningf(ctx, "扫描循环上架计划失败：%+v", err)
			}
		}
	}
}

func (s *sSysPublish) scheduleDueCyclePlans(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	lockExpiredAt := now.Add(-5 * time.Minute)
	var plans []cyclePlanRecord
	err := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Where("enabled", 1).
		Where("status", cyclePlanStatusActive).
		WhereLTE("next_run_at", now).
		Wheref("(locked_at IS NULL OR locked_at < ?)", lockExpiredAt).
		WhereNull("deleted_at").
		OrderAsc("next_run_at").
		Limit(limit).
		Scan(&plans)
	if err != nil {
		return gerror.Wrap(err, "读取到期循环上架计划失败")
	}
	for _, plan := range plans {
		runId, err := s.createCycleRunForPlan(ctx, plan, now)
		if err != nil {
			g.Log().Warningf(ctx, "创建循环上架执行记录失败 plan:%d err:%+v", plan.Id, err)
			continue
		}
		if runId > 0 {
			if err = s.enqueueCycleRun(ctx, runId, 0); err != nil {
				return gerror.Wrap(err, "循环上架执行记录入队失败")
			}
		}
	}
	return nil
}

func (s *sSysPublish) createCycleRunForPlan(ctx context.Context, plan cyclePlanRecord, now *gtime.Time) (int64, error) {
	result, err := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Where("id", plan.Id).
		Where("enabled", 1).
		Where("status", cyclePlanStatusActive).
		WhereLTE("next_run_at", now).
		Wheref("(locked_at IS NULL OR locked_at < ?)", now.Add(-5*time.Minute)).
		Data(g.Map{"locked_at": now, "updated_at": now}).
		Update()
	if err != nil {
		return 0, gerror.Wrap(err, "锁定循环上架计划失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return 0, nil
	}
	runId, err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Data(g.Map{
		"plan_id":      plan.Id,
		"tenant_id":    plan.TenantId,
		"account_id":   plan.AccountId,
		"profile_id":   plan.ProfileId,
		"task_id":      plan.TaskId,
		"status":       cycleRunStatusPending,
		"stage":        "created",
		"scheduled_at": plan.NextRunAt,
		"created_at":   now,
		"updated_at":   now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建循环上架执行记录失败")
	}
	_, _ = g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Where("id", plan.Id).
		Data(g.Map{"last_run_id": runId, "updated_at": now}).
		Update()
	return runId, nil
}

func (s *sSysPublish) ExecuteCycleRun(ctx context.Context, runId int64) error {
	run, err := s.lockCycleRun(ctx, runId)
	if err != nil {
		return err
	}
	if run.Id <= 0 {
		return nil
	}
	s.appendCycleRunLog(ctx, run, "info", "running", "循环上架开始执行", nil)
	if err = s.executeLockedCycleRun(ctx, run); err != nil {
		s.finishCycleRun(ctx, run, cycleRunStatusFailed, "failed", err.Error())
		s.appendCycleRunLog(ctx, run, "error", "failed", err.Error(), nil)
		return err
	}
	return nil
}

func (s *sSysPublish) lockCycleRun(ctx context.Context, runId int64) (cycleRunRecord, error) {
	var run cycleRunRecord
	if runId <= 0 {
		return run, nil
	}
	now := gtime.Now()
	result, err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).
		Where("id", runId).
		WhereIn("status", []string{cycleRunStatusPending, cycleRunStatusFailed}).
		Data(g.Map{"status": cycleRunStatusRunning, "stage": "running", "started_at": now, "updated_at": now}).
		Update()
	if err != nil {
		return run, gerror.Wrap(err, "锁定循环上架执行记录失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return run, nil
	}
	if err = g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", runId).Scan(&run); err != nil {
		return run, gerror.Wrap(err, "读取循环上架执行记录失败")
	}
	return run, nil
}

func (s *sSysPublish) executeLockedCycleRun(ctx context.Context, run cycleRunRecord) error {
	plan, err := s.cyclePlanById(ctx, run.PlanId)
	if err != nil {
		return err
	}
	task, err := s.cycleTaskForPlan(ctx, plan)
	if err != nil {
		return err
	}
	if plan.Id <= 0 || plan.Enabled != 1 || plan.Status != cyclePlanStatusActive || task.IsEmpty() {
		_ = s.disableCyclePlanForProfile(ctx, run.TenantId, run.AccountId, run.ProfileId)
		s.finishCycleRun(ctx, run, cycleRunStatusSkipped, "skipped", "循环计划或上架任务已失效")
		s.appendCycleRunLog(ctx, run, "info", "skipped", "循环计划或上架任务已失效", nil)
		return nil
	}
	jobs, err := s.cycleSentJobs(ctx, plan)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err = s.deleteTelegramMessageSet(ctx, job, "循环上架"); err != nil {
			return err
		}
	}
	if err = s.requeueCycleTaskTelegramJobs(ctx, plan); err != nil {
		return err
	}
	nextRunAt := s.nextCycleRunAt(ctx, plan, gtime.Now())
	now := gtime.Now()
	_, err = g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Where("id", plan.Id).
		Data(g.Map{"next_run_at": nextRunAt, "last_run_at": now, "locked_at": nil, "updated_at": now}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新下次循环上架时间失败")
	}
	s.finishCycleRun(ctx, run, cycleRunStatusSuccess, "finished", "")
	s.appendCycleRunLog(ctx, run, "info", "finished", "循环上架已重新投递", g.Map{"nextRunAt": nextRunAt})
	return nil
}

func (s *sSysPublish) cyclePlanById(ctx context.Context, planId int64) (cyclePlanRecord, error) {
	var plan cyclePlanRecord
	if planId <= 0 {
		return plan, nil
	}
	err := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Where("id", planId).
		WhereNull("deleted_at").
		Scan(&plan)
	if err != nil {
		return plan, gerror.Wrap(err, "读取循环上架计划失败")
	}
	return plan, nil
}

func (s *sSysPublish) cycleTaskForPlan(ctx context.Context, plan cyclePlanRecord) (gdb.Record, error) {
	if plan.TaskId <= 0 {
		return nil, nil
	}
	row, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("id,status,tg_status,deleted_at").
		Where("id", plan.TaskId).
		Where("profile_id", plan.ProfileId).
		Where("account_id", plan.AccountId).
		Where("status", sysin.PublishTaskStatusPublished).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取循环上架任务失败")
	}
	return row, nil
}

func (s *sSysPublish) cycleSentJobs(ctx context.Context, plan cyclePlanRecord) ([]telegramJobRecord, error) {
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", plan.TaskId).
		Where("profile_id", plan.ProfileId).
		Where("account_id", plan.AccountId).
		Where("status", "sent").
		OrderAsc("id").
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取循环上架TG任务失败")
	}
	return jobs, nil
}

func (s *sSysPublish) requeueCycleTaskTelegramJobs(ctx context.Context, plan cyclePlanRecord) error {
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", plan.TaskId).
		Where("profile_id", plan.ProfileId).
		Where("account_id", plan.AccountId).
		WhereIn("status", []string{"sent", "failed", "failed_retry"}).
		OrderAsc("id").
		Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取循环上架待重投TG任务失败")
	}
	now := gtime.Now()
	for _, job := range jobs {
		_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", job.Id).
			Data(g.Map{
				"status":        "pending",
				"retry_count":   0,
				"next_retry_at": nil,
				"error_message": "",
				"updated_at":    now,
			}).
			Update()
		if err != nil {
			return gerror.Wrap(err, "重置循环上架TG任务失败")
		}
		s.appendTelegramJobLog(ctx, job, "cycle_publish", "queued", "循环上架发布已加入队列")
		if err = s.enqueueTelegramJob(ctx, job.Id, 0); err != nil {
			return gerror.Wrap(err, "循环上架TG任务入队失败")
		}
	}
	return nil
}

func (s *sSysPublish) finishCycleRun(ctx context.Context, run cycleRunRecord, status string, stage string, errMessage string) {
	data := g.Map{
		"status":        status,
		"stage":         stage,
		"error_message": errMessage,
		"finished_at":   gtime.Now(),
		"updated_at":    gtime.Now(),
	}
	_, _ = g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", run.Id).Data(data).Update()
	if status == cycleRunStatusFailed {
		_, _ = g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
			Where("id", run.PlanId).
			Data(g.Map{"locked_at": nil, "updated_at": gtime.Now()}).
			Update()
	}
}

func (s *sSysPublish) appendCycleRunLog(ctx context.Context, run cycleRunRecord, level string, stage string, message string, contextMap g.Map) {
	var contextJSON interface{}
	if len(contextMap) > 0 {
		if data, err := json.Marshal(contextMap); err == nil {
			contextJSON = string(data)
		}
	}
	_, _ = g.DB().Model(publishCycleRunLogTable).Safe().Ctx(ctx).Data(g.Map{
		"run_id":       run.Id,
		"plan_id":      run.PlanId,
		"tenant_id":    run.TenantId,
		"account_id":   run.AccountId,
		"profile_id":   run.ProfileId,
		"level":        level,
		"stage":        stage,
		"message":      message,
		"context_json": contextJSON,
		"created_at":   gtime.Now(),
	}).Insert()
}
