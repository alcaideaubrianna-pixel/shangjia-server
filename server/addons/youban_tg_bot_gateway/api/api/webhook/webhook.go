package webhook

import "github.com/gogf/gf/v2/frame/g"

type TelegramReq struct {
	g.Meta `path:"/telegram/webhook" method:"post" tags:"TG Bot Gateway" summary:"Telegram Bot统一Webhook"`
	Key    string `json:"key" v:"required#缺少Bot Key" dc:"Bot运行Key"`
}

type TelegramRes struct{}
