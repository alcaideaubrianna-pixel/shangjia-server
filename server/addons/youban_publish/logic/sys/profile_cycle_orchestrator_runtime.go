package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// 单轮只领取 profile 引擎任务；过滤 legacy 后可适当提高吞吐，避免新账户被旧迁移数据拖慢。
const profileCycleOrchestratorDefaultBatchSize = 200

// profileCycleOrchestratorBatchSize 读取资料级调度批次；限制范围避免误配一次性冲击 Telegram。
func profileCycleOrchestratorBatchSize(ctx context.Context) int {
	batchSize := g.Cfg().MustGet(ctx, "youbanPublish.cycle.orchestratorBatchSize", profileCycleOrchestratorDefaultBatchSize).Int()
	if batchSize < 1 {
		return profileCycleOrchestratorDefaultBatchSize
	}
	if batchSize > 1000 {
		return 1000
	}
	return batchSize
}

// runProfileCycleOrchestrator 消费资料级循环 run，并复用现有资料发布入口。
// 每轮只处理有限数量，避免迁移积压一次性冲击 Telegram 发送队列。
func (s *sSysPublish) runProfileCycleOrchestrator(ctx context.Context, scopes ...publishAccountScope) error {
	if len(scopes) == 0 {
		return s.enqueueAccountSchedules(ctx)
	}
	var runs []struct {
		ID     int64  `orm:"id"`
		RunKey string `orm:"run_key"`
	}
	model := g.DB().Model(publishProfileCycleRunTable).Safe().Ctx(ctx).
		Fields("id,run_key").Where("status", "pending").WhereLTE("due_at", gtime.Now())

	model = accountScheduleModel(model, publishProfileCycleRunTable+".tenant_id", publishProfileCycleRunTable+".account_id", scopes)
	if err := model.OrderAsc("due_at").OrderAsc("id").Limit(profileCycleOrchestratorBatchSize(ctx)).Scan(&runs); err != nil {
		return err
	}
	for _, run := range runs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if run.RunKey == "" {
			continue
		}
		if err := s.enqueueProfileCyclePreparation(ctx, run.RunKey, scopes[0]); err != nil {
			return err
		}
	}
	return nil
}
