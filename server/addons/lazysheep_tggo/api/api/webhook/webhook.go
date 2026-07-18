// Package webhook
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package webhook

import "github.com/gogf/gf/v2/frame/g"

type UpdateReq struct {
	g.Meta `path:"/webhook/:botKey" method:"post" tags:"懒羊羊TGGo" summary:"Telegram webhook"`
	BotKey string `json:"botKey" p:"botKey"`
}

type UpdateRes struct {
	Ok bool `json:"ok"`
}
