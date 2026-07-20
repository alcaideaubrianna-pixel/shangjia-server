package crons

import (
	"context"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_feiniu_sync/service"
	"hotgo/internal/library/cron"
)

func init() { cron.Register(FeiniuSync) }

var FeiniuSync = &cFeiniuSync{name: "youbanFeiniuSync"}

type cFeiniuSync struct{ name string }

func (c *cFeiniuSync) GetName() string { return c.name }
func (c *cFeiniuSync) Execute(ctx context.Context, parser *cron.Parser) (err error) {
	if parser != nil && len(parser.Args) > 0 {
		configIdStr := strings.TrimSpace(parser.Args[0])
		if configIdStr != "" {
			configId, parseErr := strconv.ParseInt(configIdStr, 10, 64)
			if parseErr != nil {
				return gerror.Wrap(parseErr, "解析 FeiNiu 配置ID失败")
			}
			parser.Logger.Infof(ctx, "开始执行 FeiNiu 配置同步 configId:%d", configId)
			return service.SysSync().CronRunConfig(ctx, configId)
		}
	}
	parser.Logger.Infof(ctx, "开始执行 FeiNiu 数据同步")
	return service.SysSync().CronRun(ctx)
}
