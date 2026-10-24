package crons

import (
	"context"
	"time"

	"hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cron"
)

func init() { cron.Register(DailyReport) }

var DailyReport = &cDailyReport{name: "youbanPublishDailyReport"}

type cDailyReport struct{ name string }

func (c *cDailyReport) GetName() string { return c.name }
func (c *cDailyReport) Execute(ctx context.Context, parser *cron.Parser) error {
	if !service.SysPublish().RuntimeRoleEnabled(ctx, "scheduler") {
		return nil
	}
	if err := service.SysPublish().ProcessDailyReports(ctx, time.Now()); err != nil {
		parser.Logger.Warningf(ctx, "处理每日推送报告失败 err:%+v", err)
		return err
	}
	return nil
}
