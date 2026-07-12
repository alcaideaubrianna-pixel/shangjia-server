package bot

import (
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_bot/model/input/sysin"
)

type LoginStartReq struct {
	g.Meta `path:"/bot/login/start" method:"post" tags:"全局机器人" summary:"发起Telegram验证码登录"`
	sysin.CodeStartInp
}

type LoginStartRes struct{ *sysin.CodeStartModel }

type LoginStatusReq struct {
	g.Meta `path:"/bot/login/status" method:"get" tags:"全局机器人" summary:"查询Telegram验证码登录状态"`
	sysin.CodeStatusInp
}

type LoginStatusRes struct{ *sysin.CodeStatusModel }

type BindStartReq struct {
	g.Meta `path:"/bot/bind/start" method:"post" tags:"全局机器人" summary:"发起Telegram绑定"`
}

type BindStartRes struct{ *sysin.CodeStartModel }

type BindStatusReq struct {
	g.Meta `path:"/bot/bind/status" method:"get" tags:"全局机器人" summary:"查询Telegram绑定状态"`
	sysin.CodeStatusInp
}

type BindStatusRes struct{ *sysin.CodeStatusModel }

type BindInfoReq struct {
	g.Meta `path:"/bot/bind/info" method:"get" tags:"全局机器人" summary:"Telegram绑定信息"`
}

type BindInfoRes struct{ *sysin.BindInfoModel }

type TelegramWebhookReq struct {
	g.Meta `path:"/telegram/webhook" method:"post" tags:"全局机器人" summary:"Telegram Webhook"`
	BotId  int64 `json:"botId" dc:"Bot ID"`
}

type TelegramWebhookRes struct{}
