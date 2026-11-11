package sys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/library/profilecycle"
)

type profileCyclePreparePayload struct {
	publishAccountScope
	RunKey string `json:"runKey"`
}

// enqueueProfileCyclePreparation 只通知已提交的资料计划，复用background角色已有资料维护队列和Asynq去重，不挤占账户调度/状态投影，不增加物理队列。
func (s *sSysPublish) enqueueProfileCyclePreparation(ctx context.Context, runKey string, scope publishAccountScope) error {
	body, err := json.Marshal(profileCyclePreparePayload{publishAccountScope: scope, RunKey: runKey})
	if err != nil {
		return err
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(publishconsts.QueueProfileCyclePrepareTask, body), asynq.Queue(tgQueueNameProfileMaintenance), asynq.Unique(30*time.Second), asynq.Timeout(25*time.Second), asynq.MaxRetry(3))
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

// handleProfileCyclePreparationTask 单份资料独立准备，多个background节点可并行；SavePlan行锁保证重复投递幂等。
func (s *sSysPublish) handleProfileCyclePreparationTask(ctx context.Context, task *asynq.Task) error {
	var payload profileCyclePreparePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if payload.TenantID <= 0 || payload.AccountID <= 0 || payload.RunKey == "" {
		return asynq.SkipRetry
	}
	if s.accountPushEngine(ctx, payload.TenantID, payload.AccountID) != pushEngineUnifiedStream {
		return nil
	}
	var run struct {
		ID int64 `orm:"id"`
	}
	if err := g.DB().Model(publishProfileCycleRunTable).Ctx(ctx).Where("tenant_id", payload.TenantID).Where("account_id", payload.AccountID).Where("run_key", payload.RunKey).Where("status", "pending").WhereLTE("due_at", gtime.Now()).Fields("id").Scan(&run); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if run.ID <= 0 {
		return nil
	}
	store := s.newProfileCycleStore()
	profile, err := store.LoadProfile(ctx, payload.RunKey)
	if err == nil {
		plan := profilecycle.BuildPlan(profile, payload.RunKey, gtime.Now().Time)
		if plan == nil {
			_, err = g.DB().Model(publishProfileCycleRunTable).Ctx(ctx).Where("id", run.ID).Where("status", "pending").Data(g.Map{"status": "skipped"}).Update()
			return err
		}
		err = store.SavePlan(ctx, plan)
	}
	if err != nil {
		// 独立任务失败只退避本资料；账户的其他资料继续准备和发送。
		_, _ = g.DB().Model(publishProfileCycleRunTable).Ctx(ctx).Where("id", run.ID).Where("status", "pending").Data(g.Map{"due_at": gtime.Now().Add(profileCycleRetryDelay), "error_message": err.Error()}).Update()
	}
	return err
}
