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
	// The cron runner invokes this every day; reportDate is the just-finished
	// China calendar day, while the persisted snapshot is immutable per date.
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return err
	}
	if err = service.SysPublish().GenerateDailyReports(ctx, time.Now().In(loc).AddDate(0, 0, -1), 500); err != nil {
		parser.Logger.Warningf(ctx, "生成每日推送报告失败 err:%+v", err)
		return err
	}
	return nil
}
