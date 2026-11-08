package crons

import (
	"context"
	"os"
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
	host, _ := os.Hostname()
	parser.Logger.Infof(ctx, "循环调度 cron 开始 task:%s role:scheduler host:%s", c.name, host)
	if !service.SysPublish().RuntimeRoleEnabled(ctx, "scheduler") {
		parser.Logger.Warningf(ctx, "循环调度 cron 跳过 task:%s reason:runtime_role_disabled host:%s", c.name, host)
		return nil
	}
	startedAt := time.Now()
	if err := service.SysPublish().RunChannelCycleScheduler(ctx); err != nil {
		parser.Logger.Warningf(ctx, "cron CycleScheduler Execute err:%+v", err)
		parser.Logger.Infof(ctx, "循环调度 cron 结束 task:%s elapsed_ms:%d status:error host:%s", c.name, time.Since(startedAt).Milliseconds(), host)
		return err
	}
	parser.Logger.Infof(ctx, "循环调度 cron 结束 task:%s elapsed_ms:%d status:success host:%s", c.name, time.Since(startedAt).Milliseconds(), host)
	return nil
}
