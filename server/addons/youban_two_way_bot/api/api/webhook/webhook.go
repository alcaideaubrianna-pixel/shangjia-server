package webhook

import "github.com/gogf/gf/v2/frame/g"

type TelegramReq struct {
	g.Meta `path:"/telegram/webhook" method:"post" tags:"双向机器人" summary:"Telegram Webhook"`
	BotId  int64 `json:"botId" dc:"Bot ID"`
}

type TelegramRes struct{}
