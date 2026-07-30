package fix

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func RemoveYoubanPublishCollectRuleUniqueNo(ctx context.Context) error {
	dbType := strings.ToLower(g.DB().GetConfig().Type)
	var statement string
	switch dbType {
	case "pgsql", "postgres", "postgresql":
		statement = `ALTER TABLE "hg_youban_publish_collect_rule" DROP COLUMN IF EXISTS "show_unique_no"`
	case "mysql", "mariadb":
		statement = "ALTER TABLE `hg_youban_publish_collect_rule` DROP COLUMN IF EXISTS `show_unique_no`"
	default:
		return gerror.Newf("不支持的数据库类型:%s", dbType)
	}
	if _, err := g.DB().Exec(ctx, statement); err != nil {
		return gerror.Wrap(err, "删除采集规则唯一编号字段失败")
	}
	g.Log().Info(ctx, "采集规则唯一编号字段已删除")
	return nil
}
