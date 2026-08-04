package crons

import (
	"context"

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
	if !service.SysPublish().RuntimeRoleEnabled(ctx, "scheduler") {
		return nil
	}
	if err := service.SysPublish().RunChannelCycleScheduler(ctx); err != nil {
		parser.Logger.Warningf(ctx, "cron CycleScheduler Execute err:%+v", err)
		return err
	}
	return nil
}
