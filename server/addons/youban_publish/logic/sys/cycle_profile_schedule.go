package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	hglock "hotgo/internal/library/hgrds/lock"
)

const (
	profileCycleScanBatchSize       = 200
	profileCycleMaxProducePerRun    = 5000
	profileCycleGlobalBacklogLimit  = 3000
	profileCycleChannelBacklogLimit = 200
	profileCycleClaimLease          = 2 * time.Hour
	profileCycleRetryDelay          = time.Hour
	profileCycleRescheduleBatchSize = 500
	profileCycleOverdueSpreadWindow = time.Hour
	profileCycleSummaryRefreshDelay = 30 * time.Second
)

type profileCycleChannelConfig struct {
	Id          int64  `orm:"id"`
	TenantId    int64  `orm:"tenant_id"`
	Enabled     int    `orm:"cycle_publish_enabled"`
	Days        int    `orm:"cycle_publish_days"`
	PublishTime string `orm:"cycle_publish_time"`
	Status      int    `orm:"status"`
	Direction   string `orm:"publish_direction"`
}

type profileCycleRescheduleRow struct {
	RelationId int64       `orm:"relation_id"`
	JobId      int64       `orm:"job_id"`
	ProfileId  int64       `orm:"profile_id"`
	SentAt     *gtime.Time `orm:"sent_at"`
}

type profileCycleDueRow struct {
	JobId              int64       `orm:"job_id"`
	TenantId           int64       `orm:"tenant_id"`
	AccountId          int64       `orm:"account_id"`
	ProfileId          int64       `orm:"profile_id"`
	ChannelId          int64       `orm:"channel_id"`
	SentAt             *gtime.Time `orm:"sent_at"`
	CycleDays          int         `orm:"cycle_days"`
	CyclePublishTime   string      `orm:"cycle_publish_time"`
	NextCycleAt        *gtime.Time `orm:"next_cycle_at"`
	DispatchCount      int         `orm:"dispatch_count"`
	ChannelEnabled     int         `orm:"channel_enabled"`
	ChannelDays        int         `orm:"channel_days"`
	ChannelPublishTime string      `orm:"channel_publish_time"`
	ChannelStatus      int         `orm:"channel_status"`
	PublishDirection   string      `orm:"publish_direction"`
}

func (s *sSysPublish) syncTelegramJobCycleSchedule(ctx context.Context, job telegramJobRecord) error {
	if job.Id <= 0 || job.ChannelId <= 0 || job.ProfileId <= 0 {
		return nil
	}
	config, err := s.profileCycleChannelConfigById(ctx, job.ChannelId)
	if err != nil {
		return err
	}
	now := gtime.Now()
	if job.SentAt == nil {
		job.SentAt = now
	}
	if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("channel_id", job.ChannelId).
		Where("profile_id", job.ProfileId).
		WhereNot("id", job.Id).
		Where("cycle_enabled", 1).
		Data(g.Map{"cycle_enabled": 0, "next_cycle_at": nil, "updated_at": now}).Update(); err != nil {
		return gerror.Wrap(err, "关闭资料旧循环计划失败")
	}
	data := g.Map{
		"cycle_enabled":      0,
		"cycle_days":         defaultCycleDays(config.Days),
		"cycle_publish_time": strings.TrimSpace(config.PublishTime),
		"next_cycle_at":      nil,
		"updated_at":         now,
	}
	if profileCycleChannelUsable(config) {
		data["cycle_enabled"] = 1
		data["next_cycle_at"] = nextProfileCycleAt(ctx, config.Days, config.PublishTime, job.SentAt)
	}
	if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).WhereIn("status", []string{"sent", "superseded"}).Data(data).Update(); err != nil {
		return gerror.Wrap(err, "更新资料循环计划失败")
	}
	if err = s.enqueueCycleSummaryRefresh(ctx, job.ChannelId, profileCycleSummaryRefreshDelay); err != nil {
		g.Log().Warningf(ctx, "提交频道循环汇总刷新失败 channel:%d job:%d err:%+v", job.ChannelId, job.Id, err)
	}
	return nil
}

func (s *sSysPublish) profileCycleChannelConfigById(ctx context.Context, channelId int64) (profileCycleChannelConfig, error) {
	var config profileCycleChannelConfig
	if channelId <= 0 {
		return config, nil
	}
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,cycle_publish_enabled,cycle_publish_days,cycle_publish_time,status,publish_direction").
		Where("id", channelId).WhereNull("deleted_at").Scan(&config)
	if err != nil {
		return config, gerror.Wrap(err, "读取频道循环配置失败")
	}
	return config, nil
}

func profileCycleChannelUsable(config profileCycleChannelConfig) bool {
	return config.Id > 0 && config.Enabled == 1 && config.Status == 1 && strings.EqualFold(strings.TrimSpace(config.Direction), "up")
}

func sameProfileCycleConfig(first, second profileCycleChannelConfig) bool {
	firstUsable := profileCycleChannelUsable(first)
	secondUsable := profileCycleChannelUsable(second)
	if firstUsable != secondUsable {
		return false
	}
	if !firstUsable {
		return true
	}
	return defaultCycleDays(first.Days) == defaultCycleDays(second.Days) &&
		strings.TrimSpace(first.PublishTime) == strings.TrimSpace(second.PublishTime)
}

func nextProfileCycleAt(ctx context.Context, days int, publishTime string, sentAt *gtime.Time) *gtime.Time {
	return calculateProfileCycleAt(days, publishTime, sentAt, isDevelopMode(ctx))
}

func calculateProfileCycleAt(days int, publishTime string, sentAt *gtime.Time, develop bool) *gtime.Time {
	if sentAt == nil {
		sentAt = gtime.Now()
	}
	if develop {
		return sentAt.Add(time.Duration(defaultCycleDays(days)) * time.Second)
	}
	next := sentAt.AddDate(0, 0, defaultCycleDays(days))
	hour, minute, ok := parseCycleClock(publishTime)
	if !ok {
		return next
	}
	value := next.Time
	return gtime.New(time.Date(value.Year(), value.Month(), value.Day(), hour, minute, 0, 0, value.Location()))
}

func profileCycleOverdueAt(now *gtime.Time, profileId, channelId int64) *gtime.Time {
	if now == nil {
		now = gtime.Now()
	}
	spreadSeconds := int64(profileCycleOverdueSpreadWindow / time.Second)
	if spreadSeconds <= 0 {
		return now
	}
	offset := (profileId*31 + channelId*17) % spreadSeconds
	if offset < 0 {
		offset = -offset
	}
	return now.Add(time.Duration(offset) * time.Second)
}

func (s *sSysPublish) rescheduleChannelProfileCycles(ctx context.Context, channelId int64) error {
	if channelId <= 0 {
		return nil
	}
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock := hglock.NewConfig(2*time.Hour, 100*time.Millisecond).Mutex(fmt.Sprintf("youban_publish:cycle_reschedule:%d", channelId))
	if err := lock.TryLock(lockCtx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return gerror.New("频道循环重算正在执行，请稍后重试")
		}
		return gerror.Wrap(err, "获取频道循环重算锁失败")
	}
	defer s.releaseTelegramChannelLease(context.Background(), lock)
	config, err := s.profileCycleChannelConfigById(ctx, channelId)
	if err != nil {
		return err
	}
	now := gtime.Now()
	if !profileCycleChannelUsable(config) {
		if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("channel_id", channelId).Where("cycle_enabled", 1).
			Data(g.Map{"cycle_enabled": 0, "next_cycle_at": nil, "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "关闭频道资料循环计划失败")
		}
		return s.finishChannelProfileCycleReschedule(ctx, config)
	}
	cleanupSQL := fmt.Sprintf("UPDATE %s SET cycle_enabled=0,next_cycle_at=NULL,updated_at=? WHERE channel_id=? AND cycle_enabled=1 AND NOT EXISTS (SELECT 1 FROM %s cp WHERE cp.last_job_id=%s.id AND cp.status='active')",
		publishTgJobTable, publishChannelProfileTable, publishTgJobTable)
	if _, err = g.DB().Exec(ctx, cleanupSQL, now, channelId); err != nil {
		return gerror.Wrap(err, "清理频道历史循环计划失败")
	}
	var cursor int64
	for {
		var rows []profileCycleRescheduleRow
		err = g.DB().Model(publishChannelProfileTable+" cp").Safe().Ctx(ctx).
			InnerJoin(publishTgJobTable+" j", "j.id=cp.last_job_id AND j.status IN ('sent','superseded')").
			Fields("cp.id AS relation_id,j.id AS job_id,j.profile_id,j.sent_at").
			Where("cp.channel_id", channelId).
			Where("cp.status", "active").
			WhereGT("cp.id", cursor).
			OrderAsc("cp.id").Limit(profileCycleRescheduleBatchSize).Scan(&rows)
		if err != nil {
			return gerror.Wrap(err, "读取频道资料循环基准失败")
		}
		if len(rows) == 0 {
			break
		}
		if err = s.updateProfileCycleRescheduleBatch(ctx, config, rows, now); err != nil {
			return err
		}
		cursor = rows[len(rows)-1].RelationId
	}
	return s.finishChannelProfileCycleReschedule(ctx, config)
}

func (s *sSysPublish) finishChannelProfileCycleReschedule(ctx context.Context, config profileCycleChannelConfig) error {
	if err := s.refreshChannelProfileCycleNextAt(ctx, config.Id); err != nil {
		return err
	}
	latest, err := s.profileCycleChannelConfigById(ctx, config.Id)
	if err != nil {
		return err
	}
	if sameProfileCycleConfig(config, latest) {
		return nil
	}
	_, _ = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("id", config.Id).Data(g.Map{
		"cycle_next_run_at": nil,
		"updated_at":        gtime.Now(),
	}).Update()
	return gerror.New("频道循环配置已变化，等待按最新设置重新计算")
}

func (s *sSysPublish) updateProfileCycleRescheduleBatch(ctx context.Context, config profileCycleChannelConfig, rows []profileCycleRescheduleRow, now *gtime.Time) error {
	if len(rows) == 0 {
		return nil
	}
	caseParts := make([]string, 0, len(rows))
	idParts := make([]string, 0, len(rows))
	args := make([]any, 0, 5+len(rows)*3)
	args = append(args, defaultCycleDays(config.Days), strings.TrimSpace(config.PublishTime))
	for _, row := range rows {
		nextAt := nextProfileCycleAt(ctx, config.Days, config.PublishTime, row.SentAt)
		if nextAt == nil || !nextAt.After(now) {
			nextAt = profileCycleOverdueAt(now, row.ProfileId, config.Id)
		}
		caseParts = append(caseParts, "WHEN ? THEN ?")
		args = append(args, row.JobId, nextAt)
		idParts = append(idParts, "?")
	}
	args = append(args, now)
	for _, row := range rows {
		args = append(args, row.JobId)
	}
	sql := fmt.Sprintf("UPDATE %s SET cycle_enabled=1,cycle_days=?,cycle_publish_time=?,next_cycle_at=CASE id %s ELSE next_cycle_at END,updated_at=? WHERE id IN (%s)",
		publishTgJobTable, strings.Join(caseParts, " "), strings.Join(idParts, ","))
	if _, err := g.DB().Exec(ctx, sql, args...); err != nil {
		return gerror.Wrap(err, "批量重算频道资料循环时间失败")
	}
	return nil
}

func (s *sSysPublish) refreshChannelProfileCycleNextAt(ctx context.Context, channelId int64) error {
	if channelId <= 0 {
		return nil
	}
	value, err := g.DB().Model(publishChannelProfileTable+" cp").Safe().Ctx(ctx).
		InnerJoin(publishTgJobTable+" j", "j.id=cp.last_job_id AND j.cycle_enabled=1 AND j.status IN ('sent','superseded')").
		Where("cp.channel_id", channelId).Where("cp.status", "active").
		Fields("MIN(j.next_cycle_at)").Value()
	if err != nil {
		return gerror.Wrap(err, "刷新频道下次循环时间失败")
	}
	var nextAt any
	if !value.IsNil() {
		nextAt = value.Val()
	}
	_, err = g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Where("id", channelId).Data(g.Map{
		"cycle_next_run_at": nextAt, "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) runProfileCycleDueScan(ctx context.Context) error {
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock := hglock.NewConfig(55*time.Minute, 100*time.Millisecond).Mutex("youban_publish:profile_cycle_due_scan")
	if err := lock.TryLock(lockCtx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return nil
		}
		return gerror.Wrap(err, "获取资料循环扫描锁失败")
	}
	defer s.releaseTelegramChannelLease(context.Background(), lock)
	produced := 0
	processed := 0
	affectedChannels := make(map[int64]struct{})
	channelBacklog := make(map[int64]int)
	for produced < profileCycleMaxProducePerRun && processed < profileCycleMaxProducePerRun {
		backlog, err := s.profileCycleGlobalBacklog(ctx)
		if err != nil {
			return err
		}
		if backlog >= profileCycleGlobalBacklogLimit {
			break
		}
		rows, err := s.profileCycleDueRows(ctx, profileCycleScanBatchSize)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		processed += len(rows)
		batchProduced := 0
		for _, row := range rows {
			if produced >= profileCycleMaxProducePerRun || backlog+batchProduced >= profileCycleGlobalBacklogLimit {
				break
			}
			created, handleErr := s.dispatchDueProfileCycle(ctx, row, channelBacklog)
			if handleErr != nil {
				g.Log().Warningf(ctx, "提交到期资料循环失败 job:%d channel:%d err:%+v", row.JobId, row.ChannelId, handleErr)
				continue
			}
			affectedChannels[row.ChannelId] = struct{}{}
			if created {
				produced++
				batchProduced++
				channelBacklog[row.ChannelId]++
			}
		}
	}
	for channelId := range affectedChannels {
		if err := s.refreshChannelProfileCycleNextAt(ctx, channelId); err != nil {
			g.Log().Warningf(ctx, "刷新频道循环时间失败 channel:%d err:%+v", channelId, err)
		}
	}
	g.Log().Infof(ctx, "资料循环到期扫描完成 produced:%d channels:%d", produced, len(affectedChannels))
	return nil
}

func (s *sSysPublish) enqueuePendingProfileCycleReschedules(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 200
	}
	var channelIds []int64
	err := g.DB().Model(publishChannelTable+" c").Safe().Ctx(ctx).
		Fields("c.id").
		Where("c.cycle_publish_enabled", 1).
		Where("c.status", 1).
		Where("c.publish_direction", "up").
		WhereNull("c.cycle_next_run_at").
		WhereNull("c.deleted_at").
		Where("EXISTS (SELECT 1 FROM " + publishChannelProfileTable + " cp WHERE cp.channel_id=c.id AND cp.status='active')").
		OrderAsc("c.id").
		Limit(limit).
		Scan(&channelIds)
	if err != nil {
		return gerror.Wrap(err, "读取待恢复频道循环重算任务失败")
	}
	for _, channelId := range channelIds {
		if err = s.enqueueCycleReschedule(ctx, channelId, 0); err != nil {
			return gerror.Wrapf(err, "提交待恢复频道循环重算任务失败 channel:%d", channelId)
		}
	}
	return nil
}

func (s *sSysPublish) profileCycleDueRows(ctx context.Context, limit int) ([]profileCycleDueRow, error) {
	if limit <= 0 {
		limit = profileCycleScanBatchSize
	}
	var rows []profileCycleDueRow
	err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		InnerJoin(publishChannelProfileTable+" cp", "cp.last_job_id=j.id AND cp.status='active'").
		LeftJoin(publishChannelTable+" c", "c.id=j.channel_id AND c.deleted_at IS NULL").
		Fields("j.id AS job_id,j.tenant_id,j.account_id,j.profile_id,j.channel_id,j.sent_at,j.cycle_days,j.cycle_publish_time,j.next_cycle_at,j.dispatch_count,"+
			"COALESCE(c.cycle_publish_enabled,0) AS channel_enabled,COALESCE(c.cycle_publish_days,0) AS channel_days,"+
			"COALESCE(c.cycle_publish_time,'') AS channel_publish_time,COALESCE(c.status,0) AS channel_status,COALESCE(c.publish_direction,'') AS publish_direction").
		Where("j.cycle_enabled", 1).WhereIn("j.status", []string{"sent", "superseded"}).WhereNotNull("j.next_cycle_at").
		WhereLTE("j.next_cycle_at", gtime.Now()).OrderAsc("j.next_cycle_at").OrderAsc("j.id").Limit(limit).Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取到期资料循环计划失败")
	}
	return rows, nil
}

func (s *sSysPublish) dispatchDueProfileCycle(ctx context.Context, row profileCycleDueRow, channelBacklog map[int64]int) (bool, error) {
	config := profileCycleChannelConfig{
		Id: row.ChannelId, TenantId: row.TenantId, Enabled: row.ChannelEnabled, Days: row.ChannelDays,
		PublishTime: row.ChannelPublishTime, Status: row.ChannelStatus, Direction: row.PublishDirection,
	}
	if !profileCycleChannelUsable(config) {
		return false, s.disableProfileCycleJob(ctx, row.JobId, "频道循环配置已关闭")
	}
	if row.CycleDays != defaultCycleDays(config.Days) || strings.TrimSpace(row.CyclePublishTime) != strings.TrimSpace(config.PublishTime) {
		nextAt := nextProfileCycleAt(ctx, config.Days, config.PublishTime, row.SentAt)
		if nextAt == nil || !nextAt.After(gtime.Now()) {
			nextAt = profileCycleOverdueAt(gtime.Now(), row.ProfileId, row.ChannelId)
		}
		_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobId).Data(g.Map{
			"cycle_days": defaultCycleDays(config.Days), "cycle_publish_time": strings.TrimSpace(config.PublishTime),
			"next_cycle_at": nextAt, "updated_at": gtime.Now(),
		}).Update()
		return false, err
	}
	backlog, ok := channelBacklog[row.ChannelId]
	if !ok {
		var err error
		backlog, err = s.channelCycleBacklog(ctx, row.ChannelId)
		if err != nil {
			return false, err
		}
		channelBacklog[row.ChannelId] = backlog
	}
	if backlog >= profileCycleChannelBacklogLimit {
		_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobId).Data(g.Map{
			"next_cycle_at":       gtime.Now().Add(profileCycleRetryDelay),
			"last_dispatch_error": "频道循环队列积压，已延后领取", "updated_at": gtime.Now(),
		}).Update()
		return false, err
	}
	active, err := s.profileCycleChildActive(ctx, row.JobId)
	if err != nil {
		return false, err
	}
	if active {
		_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobId).Data(g.Map{
			"next_cycle_at": gtime.Now().Add(profileCycleClaimLease), "updated_at": gtime.Now(),
		}).Update()
		return false, err
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", row.JobId).Where("cycle_enabled", 1).WhereLTE("next_cycle_at", gtime.Now()).
		Data(g.Map{
			"next_cycle_at":       gtime.Now().Add(profileCycleClaimLease),
			"dispatched_at":       gtime.Now(),
			"dispatch_count":      gdb.Raw("dispatch_count + 1"),
			"last_dispatch_error": "",
			"updated_at":          gtime.Now(),
		}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "领取资料循环计划失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	operationNo := dueProfileCycleOperationNo(row.JobId, row.DispatchCount+1, row.ProfileId, row.ChannelId)
	err = s.submitProfilePublish(ctx, row.ProfileId, row.TenantId, row.AccountId, 0, operationNo, []int64{row.ChannelId}, true)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, errPublishProfileUnavailable) {
		_ = s.deactivateChannelProfile(ctx, row.ChannelId, row.ProfileId)
		return false, s.disableProfileCycleJob(ctx, row.JobId, "资料已下架或不可发布")
	}
	_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", row.JobId).Data(g.Map{
		"next_cycle_at":       gtime.Now().Add(profileCycleRetryDelay),
		"last_dispatch_error": err.Error(), "updated_at": gtime.Now(),
	}).Update()
	return false, err
}

func (s *sSysPublish) disableProfileCycleJob(ctx context.Context, jobId int64, message string) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Data(g.Map{
		"cycle_enabled": 0, "next_cycle_at": nil, "last_dispatch_error": strings.TrimSpace(message), "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) profileCycleChildActive(ctx context.Context, sourceJobId int64) (bool, error) {
	prefix := fmt.Sprintf("cycle_batch:due:%d:", sourceJobId)
	count, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereLike("operation_no", prefix+"%").WhereIn("status", []string{"pending", "sending", "failed_retry"}).Count()
	return count > 0, err
}

func (s *sSysPublish) profileCycleGlobalBacklog(ctx context.Context) (int, error) {
	return g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereLike("operation_no", "cycle_batch:due:%").WhereIn("status", []string{"pending", "sending", "failed_retry"}).Count()
}

func dueProfileCycleOperationNo(sourceJobId int64, attempt int, profileId int64, channelId int64) string {
	return fmt.Sprintf("cycle_batch:due:%d:%d:%d:%d", sourceJobId, attempt, profileId, channelId)
}

func (s *sSysPublish) runProfileCycleStartupRecovery(ctx context.Context) {
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	var channelIds []int64
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id").Where("cycle_publish_enabled", 1).Where("status", 1).
		Where("publish_direction", "up").WhereNull("deleted_at").Scan(&channelIds); err != nil {
		g.Log().Warningf(ctx, "读取启动循环恢复频道失败：%+v", err)
	} else {
		for _, channelId := range channelIds {
			if err := s.enqueueCycleReschedule(ctx, channelId, 0); err != nil {
				g.Log().Warningf(ctx, "提交启动循环重算失败 channel:%d err:%+v", channelId, err)
			}
		}
	}
	if err := s.runProfileCycleDueScan(ctx); err != nil && ctx.Err() == nil {
		g.Log().Warningf(ctx, "启动资料循环补扫失败：%+v", err)
	}
}
