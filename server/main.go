// Package main
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package main

import (
	_ "hotgo/internal/packed"

	_ "github.com/gogf/gf/contrib/drivers/mysql/v2"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gctx"
	_ "hotgo/addons/modules"
	"hotgo/internal/bootstrap"
	"hotgo/internal/bootstrap/envconfig"
	"hotgo/internal/cmd"
	"hotgo/internal/global"
	_ "hotgo/internal/logic"
)

func main() {
	var ctx = gctx.GetInitCtx()
	envconfig.Apply(ctx)
	if err := bootstrap.InitDatabaseFromEnv(ctx); err != nil {
		g.Log().Fatalf(ctx, "初始化数据库失败: %+v", err)
	}
	global.Init(ctx)
	cmd.Main.Run(ctx)
}
