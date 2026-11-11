package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/library/profilecycle"
)

const (
	publishProfileCycleRunTable        = "hg_youban_publish_profile_cycle_run"
	publishProfileCycleRunChannelTable = "hg_youban_publish_profile_cycle_run_channel"
)

// profileCycleStore 将资料级 run 适配到现有资料发布和 Outbox 能力。
type profileCycleStore struct{ publish *sSysPublish }

func (s *sSysPublish) newProfileCycleStore() profilecycle.Store {
	return &profileCycleStore{publish: s}
}

func (s *profileCycleStore) LoadProfile(ctx context.Context, runKey string) (profilecycle.Profile, error) {
	var run struct {
		TenantID  int64       `orm:"tenant_id"`
		AccountID int64       `orm:"account_id"`
		ProfileID int64       `orm:"profile_id"`
		DueAt     *gtime.Time `orm:"due_at"`
	}
	if err := g.DB().Model(publishProfileCycleRunTable).Safe().Ctx(ctx).Fields("tenant_id,account_id,profile_id,due_at").Where("run_key", runKey).Scan(&run); err != nil {
		return profilecycle.Profile{}, err
	}
	var rows []struct {
		ChannelID int64  `orm:"channel_id"`
		Enabled   int    `orm:"cycle_publish_enabled"`
		Status    int    `orm:"status"`
		Direction string `orm:"publish_direction"`
		Mode      string `orm:"cycle_publish_mode"`
		Days      int    `orm:"cycle_publish_days"`
	}
	if err := g.DB().Model(publishChannelProfileTable+" cp").Safe().Ctx(ctx).
		InnerJoin(publishChannelTable+" c", "c.id=cp.channel_id AND c.deleted_at IS NULL").
		Fields("c.id AS channel_id,c.cycle_publish_enabled,c.status,c.publish_direction,c.cycle_publish_mode,c.cycle_publish_days").
		Where("cp.profile_id", run.ProfileID).Where("cp.status", "active").
		Where("cp.tenant_id", run.TenantID).Where("cp.account_id", run.AccountID).
		Where("EXISTS (SELECT 1 FROM "+publishProfileCycleRunChannelTable+" rc JOIN "+publishProfileCycleRunTable+" r ON r.id=rc.run_id WHERE r.run_key=? AND rc.channel_id=c.id)", runKey).Scan(&rows); err != nil {
		return profilecycle.Profile{}, err
	}
	channels := make([]profilecycle.Channel, 0, len(rows))
	for _, row := range rows {
		channels = append(channels, profilecycle.Channel{ID: row.ChannelID, Enabled: row.Enabled == 1, Status: row.Status, Direction: row.Direction, Mode: strings.ToLower(strings.TrimSpace(row.Mode)), Interval: cycleDaysDuration(defaultCycleDays(row.Days))})
	}
	var dueAt time.Time
	if run.DueAt != nil {
		if parsed := gtime.NewFromStr(run.DueAt.Format("Y-m-d H:i:s")); parsed != nil {
			dueAt = parsed.Time
		}
	}
	return profilecycle.Profile{ID: run.ProfileID, TenantID: run.TenantID, AccountID: run.AccountID, Channels: channels, DueAt: dueAt}, nil
}

func (s *profileCycleStore) SavePlan(ctx context.Context, plan *profilecycle.RunPlan) error {
	if plan == nil || s.publish == nil {
		return profilecycle.ErrInvalidOrchestrator
	}
	var jobIDs []int64
	// 同一资料的一轮循环只准备一次；任务、频道映射和运行状态在同一事务提交。
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var run struct {
			ID     int64  `orm:"id"`
			Status string `orm:"status"`
		}
		if err := tx.Model(publishProfileCycleRunTable).Where("run_key", plan.RunID).
			Where("tenant_id", plan.TenantID).Where("account_id", plan.AccountID).LockUpdate().Scan(&run); err != nil {
			return err
		}
		if run.ID <= 0 {
			return gerror.New("资料循环 run 不存在")
		}
		if run.Status != "pending" {
			return nil
		}
		var existing []struct {
			ChannelID int64 `orm:"channel_id"`
			JobID     int64 `orm:"job_id"`
		}
		if err := tx.Model(publishProfileCycleRunChannelTable).Where("run_id", run.ID).Fields("channel_id,job_id").Scan(&existing); err != nil {
			return err
		}
		bound := make(map[int64]int64, len(existing))
		for _, row := range existing {
			bound[row.ChannelID] = row.JobID
		}
		channels := make([]int64, 0, len(plan.Tasks))
		for _, task := range plan.Tasks {
			if bound[task.ChannelID] <= 0 {
				channels = append(channels, task.ChannelID)
			}
		}
		if len(channels) > 0 {
			var err error
			jobIDs, err = s.publish.prepareProfilePublishJobs(ctx, plan.ProfileID, plan.TenantID, plan.AccountID,
				fmt.Sprintf("profile_cycle:%s", plan.RunID), channels, true, telegramProfilePublishMeta{})
			if err != nil {
				return err
			}
			var jobs []struct {
				ID           int64  `orm:"id"`
				ChannelID    int64  `orm:"channel_id"`
				Status       string `orm:"status"`
				ErrorMessage string `orm:"error_message"`
			}
			if err := tx.Model(publishTgJobTable).WhereIn("id", jobIDs).Fields("id,channel_id,status,error_message").Scan(&jobs); err != nil {
				return err
			}
			for _, job := range jobs {
				state := "queued"
				if job.Status == telegramJobStatusFailed {
					state = "failed"
				}
				if _, err := tx.Model(publishProfileCycleRunChannelTable).Where("run_id", run.ID).Where("channel_id", job.ChannelID).
					Data(g.Map{"job_id": job.ID, "status": state, "error_message": job.ErrorMessage}).Update(); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Model(publishProfileCycleRunChannelTable).Where("run_id", run.ID).Where("job_id", 0).Data(g.Map{"status": "skipped", "error_message": "频道已关闭或权限暂停"}).Update(); err != nil {
			return err
		}
		if _, err := tx.Model(publishProfileCycleRunTable).Where("id", run.ID).Data(g.Map{"status": "dispatched", "claimed_at": gtime.Now()}).Update(); err != nil {
			return err
		}
		return finishProfileCycleRun(ctx, run.ID)
	})
	if err != nil {
		return err
	}
	s.publish.enqueuePreparedProfileJobs(ctx, jobIDs, 0)
	return nil
}

func cycleDaysDuration(days int) time.Duration {
	if days <= 0 {
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

var _ profilecycle.Store = (*profileCycleStore)(nil)
