package crons

import (
	"context"
	"time"

	"hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cron"
)

func init() {
	cron.Register(CycleScheduler)
}

var CycleScheduler = &cCycleScheduler{name: "youbanPublishCycleScheduler"}

type cCycleScheduler struct {
	name string
}

func (c *cCycleScheduler) GetName() string {
	return c.name
}

func (c *cCycleScheduler) Execute(ctx context.Context, parser *cron.Parser) error {
	parser.Logger.Infof(ctx, "循环调度 cron 开始 task:%s role:scheduler", c.name)
	if !service.SysPublish().RuntimeRoleEnabled(ctx, "scheduler") {
		parser.Logger.Warningf(ctx, "循环调度 cron 跳过 task:%s reason:runtime_role_disabled", c.name)
		return nil
	}
	startedAt := time.Now()
	if err := service.SysPublish().RunChannelCycleScheduler(ctx); err != nil {
		parser.Logger.Warningf(ctx, "cron CycleScheduler Execute err:%+v", err)
		parser.Logger.Infof(ctx, "循环调度 cron 结束 task:%s elapsed_ms:%d status:error", c.name, time.Since(startedAt).Milliseconds())
		return err
	}
	parser.Logger.Infof(ctx, "循环调度 cron 结束 task:%s elapsed_ms:%d status:success", c.name, time.Since(startedAt).Milliseconds())
	return nil
}
