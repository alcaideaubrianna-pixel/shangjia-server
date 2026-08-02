package crons

import (
	"context"

	"hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cron"
)

func init() {
	cron.Register(VipLifecycle)
}

var VipLifecycle = &cVipLifecycle{name: "youbanPublishVipLifecycle"}

type cVipLifecycle struct {
	name string
}

func (c *cVipLifecycle) GetName() string {
	return c.name
}

func (c *cVipLifecycle) Execute(ctx context.Context, parser *cron.Parser) error {
	if err := service.SysPublish().ProcessTenantVipLifecycle(ctx, 500); err != nil {
		parser.Logger.Warningf(ctx, "cron VipLifecycle Execute err:%+v", err)
		return err
	}
	return nil
}
