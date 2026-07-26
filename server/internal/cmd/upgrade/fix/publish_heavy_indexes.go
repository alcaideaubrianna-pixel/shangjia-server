package fix

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/install"
)

// ApplyYoubanPublishHeavyIndexes creates large publish indexes outside the
// addon upgrade request path. PostgreSQL uses concurrent index creation, so it
// must not run inside the normal installer transaction.
func ApplyYoubanPublishHeavyIndexes(ctx context.Context) error {
	g.Log().Info(ctx, "开始创建上架系统大表索引")
	if err := install.ExecMaintenanceSql(ctx, install.HeavyIndexSqlFiles()); err != nil {
		return err
	}
	g.Log().Info(ctx, "上架系统大表索引创建完成")
	return nil
}
