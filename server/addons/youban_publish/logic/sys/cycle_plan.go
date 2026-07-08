package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type publishCyclePlan struct {
	job telegramJobRecord
}

func newPublishCyclePlan(job telegramJobRecord) publishCyclePlan {
	return publishCyclePlan{job: job}
}

func (p publishCyclePlan) Enabled() bool {
	return p.job.CycleEnabled == 1
}

func (p publishCyclePlan) DeleteDelay(ctx context.Context, now *gtime.Time) time.Duration {
	if !p.Enabled() {
		return 0
	}
	if now == nil {
		now = gtime.Now()
	}
	if isDevelopMode(ctx) {
		return time.Duration(defaultCycleDays(p.job.CycleDays)) * time.Second
	}
	nextAt := p.NextDeleteAt(now)
	delay := nextAt.Sub(now)
	if delay <= 0 {
		return 0
	}
	return delay
}

func (p publishCyclePlan) NextDeleteAt(now *gtime.Time) *gtime.Time {
	if now == nil {
		now = gtime.Now()
	}
	next := now.AddDate(0, 0, defaultCycleDays(p.job.CycleDays))
	hour, minute, ok := parseCycleClock(p.job.CyclePublishTime)
	if !ok {
		return next
	}
	base := next.Time
	return gtime.New(time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, base.Location()))
}

func (p publishCyclePlan) DueDelay(now *gtime.Time) time.Duration {
	if now == nil {
		now = gtime.Now()
	}
	if p.job.NextCycleAt == nil {
		return 0
	}
	nextAt := time.Date(
		p.job.NextCycleAt.Year(),
		time.Month(p.job.NextCycleAt.Month()),
		p.job.NextCycleAt.Day(),
		p.job.NextCycleAt.Hour(),
		p.job.NextCycleAt.Minute(),
		p.job.NextCycleAt.Second(),
		p.job.NextCycleAt.Nanosecond(),
		now.Location(),
	)
	delay := nextAt.Sub(now.Time)
	if delay <= 0 {
		return 0
	}
	return delay
}

func (p publishCyclePlan) CanRepublish(task gdb.Record) bool {
	return p.Enabled() && p.job.Status == "sent" && !task.IsEmpty() && task["status"].String() == sysin.PublishTaskStatusPublished
}

func parseCycleClock(value string) (int, int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, false
	}
	parts := strings.Split(value, ":")
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

func (s *sSysPublish) cycleTaskForJob(ctx context.Context, job telegramJobRecord) (gdb.Record, error) {
	if job.TaskId <= 0 {
		return nil, gerror.New("TG任务缺少上架任务ID")
	}
	row, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("id,status,tg_status,tg_operation_no,deleted_at").
		Where("id", job.TaskId).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取循环上架任务失败")
	}
	if row.IsEmpty() {
		return nil, nil
	}
	return row, nil
}

func cycleSkipMessage(job telegramJobRecord, task gdb.Record) string {
	if task.IsEmpty() {
		return fmt.Sprintf("循环上架已跳过，任务不存在或已删除，job:%d", job.Id)
	}
	if job.Status != "sent" {
		return fmt.Sprintf("循环上架已跳过，TG任务状态:%s，job:%d", job.Status, job.Id)
	}
	return fmt.Sprintf("循环上架已跳过，任务状态:%s，job:%d", task["status"].String(), job.Id)
}
