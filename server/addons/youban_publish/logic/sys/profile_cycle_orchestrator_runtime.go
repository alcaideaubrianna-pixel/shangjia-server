package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/library/profilecycle"
)

const profileCycleOrchestratorBatchSize = 20

// runProfileCycleOrchestrator 消费资料级循环 run，并复用现有资料发布入口。
// 每轮只处理有限数量，避免迁移积压一次性冲击 Telegram 发送队列。
func (s *sSysPublish) runProfileCycleOrchestrator(ctx context.Context) error {
	var runs []struct {
		ID     int64  `orm:"id"`
		RunKey string `orm:"run_key"`
	}
	if err := g.DB().Model(publishProfileCycleRunTable).Safe().Ctx(ctx).
		Fields("id,run_key").Where("status", "pending").WhereLTE("due_at", gtime.Now()).
		OrderAsc("due_at").OrderAsc("id").Limit(profileCycleOrchestratorBatchSize).Scan(&runs); err != nil {
		return err
	}
	store := s.newProfileCycleStore()
	for _, run := range runs {
		if run.RunKey == "" {
			continue
		}
		profile, err := store.LoadProfile(ctx, run.RunKey)
		if err != nil {
			g.Log().Warningf(ctx, "读取资料级循环资料失败 run:%s err:%+v", run.RunKey, err)
			continue
		}
		plan := profilecycle.BuildPlan(profile, run.RunKey, gtime.Now().Time)
		if plan == nil {
			_, _ = g.DB().Model(publishProfileCycleRunTable).Safe().Ctx(ctx).Where("id", run.ID).Data(g.Map{"status": "skipped", "updated_at": gtime.Now()}).Update()
			continue
		}
		if err = store.SavePlan(ctx, plan); err != nil {
			g.Log().Warningf(ctx, "创建资料级循环任务失败 run:%s profile:%d err:%+v", run.RunKey, profile.ID, err)
			continue
		}
		_, _ = g.DB().Model(publishProfileCycleRunTable).Safe().Ctx(ctx).Where("id", run.ID).Where("status", "pending").Data(g.Map{"status": "dispatched", "claimed_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	}
	return nil
}
