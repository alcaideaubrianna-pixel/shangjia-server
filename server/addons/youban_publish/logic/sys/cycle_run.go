package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	cycleRunStatusPending  = "pending"
	cycleRunStatusRunning  = "running"
	cycleRunStatusFinished = "finished"
	cycleRunStatusFailed   = "failed"
	cycleRunStatusSkipped  = "skipped"
	cycleBatchPageSize     = 200
	cycleBatchBacklogLimit = 1000
)

type channelCycleRecord struct {
	Id               int64       `orm:"id"`
	TenantId         int64       `orm:"tenant_id"`
	Enabled          int         `orm:"cycle_publish_enabled"`
	Days             int         `orm:"cycle_publish_days"`
	PublishTime      string      `orm:"cycle_publish_time"`
	NextRunAt        *gtime.Time `orm:"cycle_next_run_at"`
	ActiveRunId      int64       `orm:"cycle_active_run_id"`
	Status           int         `orm:"status"`
	PublishDirection string      `orm:"publish_direction"`
}

type cycleRunRecord struct {
	Id           int64       `orm:"id"`
	TenantId     int64       `orm:"tenant_id"`
	ChannelId    int64       `orm:"channel_id"`
	Status       string      `orm:"status"`
	Stage        string      `orm:"stage"`
	CursorId     int64       `orm:"cursor_id"`
	TotalCount   int         `orm:"total_count"`
	QueuedCount  int         `orm:"queued_count"`
	ScheduledAt  *gtime.Time `orm:"scheduled_at"`
	StartedAt    *gtime.Time `orm:"started_at"`
	FinishedAt   *gtime.Time `orm:"finished_at"`
	ErrorMessage string      `orm:"error_message"`
}

type channelProfileRecord struct {
	Id        int64 `orm:"id"`
	TenantId  int64 `orm:"tenant_id"`
	AccountId int64 `orm:"account_id"`
	ChannelId int64 `orm:"channel_id"`
	ProfileId int64 `orm:"profile_id"`
}

func (s *sSysPublish) RunChannelCycleScheduler(ctx context.Context) error {
	if err := s.initializeChannelCycleSchedules(ctx); err != nil {
		return err
	}
	if err := s.recoverChannelCycleRuns(ctx); err != nil {
		return err
	}
	if err := s.backfillChannelProfiles(ctx, 5000); err != nil {
		return err
	}
	pending, err := s.channelProfileBackfillPending(ctx)
	if err != nil {
		return err
	}
	if pending {
		return nil
	}
	var channels []channelCycleRecord
	if err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,cycle_publish_enabled,cycle_publish_days,cycle_publish_time,cycle_next_run_at,cycle_active_run_id,status,publish_direction").
		Where("cycle_publish_enabled", 1).
		Where("status", 1).
		Where("publish_direction", "up").
		Where("cycle_active_run_id", 0).
		WhereLTE("cycle_next_run_at", gtime.Now()).
		WhereNull("deleted_at").
		OrderAsc("cycle_next_run_at").
		Limit(20).
		Scan(&channels); err != nil {
		return gerror.Wrap(err, "读取到期频道循环任务失败")
	}
	for _, channel := range channels {
		runId, err := s.createChannelCycleRun(ctx, channel)
		if err != nil {
			g.Log().Warningf(ctx, "创建频道循环批次失败 channel:%d err:%+v", channel.Id, err)
			continue
		}
		if runId <= 0 {
			continue
		}
		if err = s.enqueueCycleRun(ctx, runId, 0); err != nil {
			s.failChannelCycleRun(ctx, runId, channel.Id, err)
			return gerror.Wrap(err, "频道循环批次入队失败")
		}
	}
	return nil
}

func (s *sSysPublish) recoverChannelCycleRuns(ctx context.Context) error {
	now := gtime.Now()
	staleRunningBefore := now.Add(-35 * time.Minute)
	var staleRunning []cycleRunRecord
	if err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).
		Fields("id,channel_id").
		Where("status", cycleRunStatusRunning).
		WhereLTE("updated_at", staleRunningBefore).
		OrderAsc("id").
		Limit(20).
		Scan(&staleRunning); err != nil {
		return gerror.Wrap(err, "读取超时频道循环批次失败")
	}
	for _, run := range staleRunning {
		result, err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).
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

	var waiting []cycleRunRecord
	if err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).
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

func (s *sSysPublish) initializeChannelCycleSchedules(ctx context.Context) error {
	var channels []channelCycleRecord
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,cycle_publish_enabled,cycle_publish_days,cycle_publish_time,cycle_next_run_at,cycle_active_run_id,status,publish_direction").
		Where("cycle_publish_enabled", 1).
		Where("status", 1).
		Where("publish_direction", "up").
		WhereNull("cycle_next_run_at").
		WhereNull("deleted_at").
		Limit(100).
		Scan(&channels); err != nil {
		return gerror.Wrap(err, "读取待初始化频道循环时间失败")
	}
	for _, channel := range channels {
		nextRunAt := s.nextChannelCycleRunAt(ctx, channel.Days, channel.PublishTime, gtime.Now())
		if _, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
			Where("id", channel.Id).
			WhereNull("cycle_next_run_at").
			Data(g.Map{"cycle_next_run_at": nextRunAt, "updated_at": gtime.Now()}).
			Update(); err != nil {
			return gerror.Wrap(err, "初始化频道循环时间失败")
		}
	}
	return nil
}

func (s *sSysPublish) createChannelCycleRun(ctx context.Context, channel channelCycleRecord) (runId int64, err error) {
	if channel.Id <= 0 || channel.NextRunAt == nil {
		return 0, nil
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		result, updateErr := tx.Model(publishChannelTable).Ctx(ctx).
			Where("id", channel.Id).
			Where("cycle_publish_enabled", 1).
			Where("cycle_active_run_id", 0).
			WhereLTE("cycle_next_run_at", now).
			WhereNull("deleted_at").
			Data(g.Map{"cycle_active_run_id": -1, "updated_at": now}).
			Update()
		if updateErr != nil {
			return gerror.Wrap(updateErr, "锁定频道循环配置失败")
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return nil
		}
		totalCount, countErr := tx.Model(publishChannelProfileTable).Ctx(ctx).
			Where("channel_id", channel.Id).
			Where("status", "active").
			Count()
		if countErr != nil {
			return gerror.Wrap(countErr, "统计频道循环资料失败")
		}
		runId, updateErr = tx.Model(publishCycleRunTable).Ctx(ctx).Data(g.Map{
			"plan_id": 0, "tenant_id": channel.TenantId, "account_id": 0,
			"profile_id": 0, "channel_id": channel.Id, "task_id": 0,
			"status": cycleRunStatusPending, "stage": "created",
			"cursor_id": 0, "total_count": totalCount, "queued_count": 0,
			"scheduled_at": channel.NextRunAt, "created_at": now, "updated_at": now,
		}).InsertAndGetId()
		if updateErr != nil {
			return gerror.Wrap(updateErr, "创建频道循环批次失败")
		}
		nextRunAt := s.nextChannelCycleRunAt(ctx, channel.Days, channel.PublishTime, channel.NextRunAt)
		_, updateErr = tx.Model(publishChannelTable).Ctx(ctx).Where("id", channel.Id).Data(g.Map{
			"cycle_active_run_id": runId,
			"cycle_next_run_at":   nextRunAt,
			"updated_at":          now,
		}).Update()
		return updateErr
	})
	return runId, err
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
	backlog, err := s.channelCycleBacklog(ctx, run.ChannelId)
	if err != nil {
		s.failChannelCycleRun(ctx, run.Id, run.ChannelId, err)
		return err
	}
	if backlog >= cycleBatchBacklogLimit {
		return s.continueChannelCycleRun(ctx, run, 30*time.Second)
	}
	items, err := s.channelCyclePage(ctx, run.ChannelId, run.CursorId, cycleBatchPageSize)
	if err != nil {
		s.failChannelCycleRun(ctx, run.Id, run.ChannelId, err)
		return err
	}
	if len(items) == 0 {
		s.finishChannelCycleRun(ctx, run, cycleRunStatusFinished, "")
		return nil
	}
	lastCursor := run.CursorId
	queued := 0
	for _, item := range items {
		if item.Id > lastCursor {
			lastCursor = item.Id
		}
		created, enqueueErr := s.enqueueChannelCycleProfile(ctx, run.Id, item)
		if enqueueErr != nil {
			err = enqueueErr
			s.failChannelCycleRun(ctx, run.Id, run.ChannelId, err)
			return err
		}
		if created {
			queued++
		}
	}
	_, err = g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", run.Id).Data(g.Map{
		"cursor_id": lastCursor, "queued_count": run.QueuedCount + queued,
		"status": cycleRunStatusPending, "stage": "producing", "error_message": "", "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新频道循环批次游标失败")
	}
	return s.enqueueCycleRun(ctx, run.Id, time.Second)
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
		Fields("id,tenant_id,cycle_publish_enabled,cycle_publish_days,cycle_publish_time,cycle_next_run_at,cycle_active_run_id,status,publish_direction").
		Where("id", channelId).
		WhereNull("deleted_at").
		Scan(&channel)
	return channel, err
}

func (s *sSysPublish) channelCyclePage(ctx context.Context, channelId int64, cursorId int64, limit int) ([]channelProfileRecord, error) {
	var items []channelProfileRecord
	err := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,channel_id,profile_id").
		Where("channel_id", channelId).
		Where("status", "active").
		WhereGT("id", cursorId).
		OrderAsc("id").
		Limit(limit).
		Scan(&items)
	if err != nil {
		return nil, gerror.Wrap(err, "分页读取频道循环资料失败")
	}
	return items, nil
}

func (s *sSysPublish) channelCycleBacklog(ctx context.Context, channelId int64) (int, error) {
	return g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("channel_id", channelId).
		WhereLike("operation_no", "cycle_batch:%").
		WhereIn("status", []string{"pending", "sending", "failed_retry"}).
		Count()
}

func (s *sSysPublish) enqueueChannelCycleProfile(ctx context.Context, runId int64, item channelProfileRecord) (bool, error) {
	clientRequestId := cyclePublishClientRequestID(runId, item.ProfileId, item.ChannelId)
	taskId, skipSubmit, err := s.createProfilePublishSnapshot(ctx, item.ProfileId, item.TenantId, item.AccountId, profilePublishSnapshotOptions{
		ChannelIds:      []int64{item.ChannelId},
		ClientRequestId: clientRequestId,
		RequireOnline:   true,
	})
	if err != nil {
		if errors.Is(err, errPublishProfileUnavailable) {
			return false, s.deactivateChannelProfile(ctx, item.ChannelId, item.ProfileId)
		}
		return false, err
	}
	if skipSubmit || taskId <= 0 {
		return false, nil
	}
	operationNo := cyclePublishOperationNo(runId, item.ProfileId, item.ChannelId)
	if err = s.submitPublishEvent(ctx, taskId, item.TenantId, 0, operationNo); err != nil {
		return false, err
	}
	return true, nil
}

func cyclePublishClientRequestID(runId int64, profileId int64, channelId int64) string {
	return fmt.Sprintf("cycle:%d:%d:%d", runId, profileId, channelId)
}

func cyclePublishOperationNo(runId int64, profileId int64, channelId int64) string {
	return fmt.Sprintf("cycle_batch:%d:%d:%d", runId, profileId, channelId)
}

func (s *sSysPublish) continueChannelCycleRun(ctx context.Context, run cycleRunRecord, delay time.Duration) error {
	_, err := g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", run.Id).Data(g.Map{
		"status": cycleRunStatusPending, "stage": "backpressure", "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return err
	}
	return s.enqueueCycleRun(ctx, run.Id, delay)
}

func (s *sSysPublish) finishChannelCycleRun(ctx context.Context, run cycleRunRecord, status string, message string) {
	now := gtime.Now()
	_, _ = g.DB().Model(publishCycleRunTable).Safe().Ctx(ctx).Where("id", run.Id).Data(g.Map{
		"status": status, "stage": "finished", "error_message": message, "finished_at": now, "updated_at": now,
	}).Update()
	_, _ = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("id", run.ChannelId).Where("cycle_active_run_id", run.Id).Data(g.Map{
		"cycle_active_run_id": 0, "cycle_last_run_at": now, "cycle_last_error_message": message, "updated_at": now,
	}).Update()
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

func (s *sSysPublish) nextChannelCycleRunAt(ctx context.Context, days int, publishTime string, base *gtime.Time) *gtime.Time {
	if base == nil {
		base = gtime.Now()
	}
	if isDevelopMode(ctx) {
		return base.Add(time.Duration(defaultCycleDays(days)) * time.Second)
	}
	next := base.AddDate(0, 0, defaultCycleDays(days))
	hour, minute, ok := parseCycleClock(publishTime)
	if !ok {
		return next
	}
	value := next.Time
	return gtime.New(time.Date(value.Year(), value.Month(), value.Day(), hour, minute, 0, 0, value.Location()))
}
