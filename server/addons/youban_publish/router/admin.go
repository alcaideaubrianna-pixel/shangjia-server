package router

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/addons/youban_publish/router/genrouter"
)

func Admin(ctx context.Context, group *ghttp.RouterGroup) {
	genrouter.Register(ctx, group)
}
