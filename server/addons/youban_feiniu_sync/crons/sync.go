package crons

import (
	"context"
	"hotgo/addons/youban_feiniu_sync/service"
	"hotgo/internal/library/cron"
)

func init() { cron.Register(FeiniuSync) }

var FeiniuSync = &cFeiniuSync{name: "youbanFeiniuSync"}

type cFeiniuSync struct{ name string }

func (c *cFeiniuSync) GetName() string { return c.name }
func (c *cFeiniuSync) Execute(ctx context.Context, parser *cron.Parser) (err error) {
	parser.Logger.Infof(ctx, "开始执行 FeiNiu 数据同步")
	return service.SysSync().CronRun(ctx)
}
