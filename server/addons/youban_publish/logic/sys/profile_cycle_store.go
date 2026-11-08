package sys

import (
	"context"
	"fmt"
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
		TenantID  int64 `orm:"tenant_id"`
		AccountID int64 `orm:"account_id"`
		ProfileID int64 `orm:"profile_id"`
	}
	if err := g.DB().Model(publishProfileCycleRunTable).Safe().Ctx(ctx).Fields("tenant_id,account_id,profile_id").Where("run_key", runKey).Scan(&run); err != nil {
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
		Where("cp.profile_id", run.ProfileID).Where("cp.status", "active").Scan(&rows); err != nil {
		return profilecycle.Profile{}, err
	}
	channels := make([]profilecycle.Channel, 0, len(rows))
	for _, row := range rows {
		channels = append(channels, profilecycle.Channel{ID: row.ChannelID, Enabled: row.Enabled == 1, Status: row.Status, Direction: row.Direction, Mode: row.Mode, Interval: cycleDaysDuration(row.Days)})
	}
	return profilecycle.Profile{ID: run.ProfileID, TenantID: run.TenantID, AccountID: run.AccountID, Channels: channels, LastSent: gtime.Now().Time}, nil
}

func (s *profileCycleStore) SavePlan(ctx context.Context, plan *profilecycle.RunPlan) error {
	if plan == nil || s.publish == nil {
		return profilecycle.ErrInvalidOrchestrator
	}
	// RunPlan.RunID 是业务 run_key；子任务 run_id 是 bigint 主键，先解析真实主键。
	runID, err := g.DB().Model(publishProfileCycleRunTable).Safe().Ctx(ctx).
		Where("run_key", plan.RunID).Value("id")
	if err != nil {
		return gerror.Wrap(err, "读取资料循环 run 主键失败")
	}
	if runID.Int64() <= 0 {
		return gerror.New("资料循环 run 不存在")
	}
	for _, task := range plan.Tasks {
		var existing int64
		if value, readErr := g.DB().Model(publishProfileCycleRunChannelTable).Safe().Ctx(ctx).Where("run_id", runID.Int64()).Where("channel_id", task.ChannelID).Value("job_id"); readErr == nil {
			existing = value.Int64()
		}
		if existing > 0 {
			continue
		}
		operationNo := fmt.Sprintf("profile_cycle:%s:%d", plan.RunID, task.ChannelID)
		jobIDs, err := s.publish.prepareProfilePublishJobs(ctx, plan.ProfileID, plan.TenantID, plan.AccountID, operationNo, []int64{task.ChannelID}, true, telegramProfilePublishMeta{})
		if err != nil {
			return err
		}
		if len(jobIDs) == 0 {
			continue
		}
		if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			_, updateErr := tx.Model(publishProfileCycleRunChannelTable).Safe().Ctx(ctx).Where("run_id", runID.Int64()).Where("channel_id", task.ChannelID).Data(g.Map{"job_id": jobIDs[0], "status": "queued", "updated_at": gtime.Now()}).Update()
			return updateErr
		}); err != nil {
			return err
		}
		s.publish.enqueuePreparedProfileJobs(ctx, jobIDs, 0)
	}
	return nil
}

func cycleDaysDuration(days int) time.Duration {
	if days <= 0 {
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

var _ profilecycle.Store = (*profileCycleStore)(nil)
