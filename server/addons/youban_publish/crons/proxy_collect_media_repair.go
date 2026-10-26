package crons

import (
	"context"

	"hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cron"
)

func init() { cron.Register(ProxyCollectMediaRepair) }

var ProxyCollectMediaRepair = &cProxyCollectMediaRepair{name: "youbanPublishProxyCollectMediaRepair"}

type cProxyCollectMediaRepair struct{ name string }

func (c *cProxyCollectMediaRepair) GetName() string { return c.name }

func (c *cProxyCollectMediaRepair) Execute(ctx context.Context, parser *cron.Parser) error {
	if !service.SysPublish().RuntimeRoleEnabled(ctx, "scheduler") {
		return nil
	}
	processed, repaired, err := service.SysPublish().ProcessProxyCollectMediaRepairs(ctx, 5)
	if err != nil {
		parser.Logger.Warningf(ctx, "代理采集缺失媒体修复失败 processed:%d repaired:%d err:%+v", processed, repaired, err)
		return err
	}
	parser.Logger.Infof(ctx, "代理采集缺失媒体修复完成 processed:%d repaired:%d", processed, repaired)
	return nil
}
