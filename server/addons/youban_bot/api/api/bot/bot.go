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

type CustomEmojiResolveReq struct {
	g.Meta `path:"/bot/custom-emoji/resolve" method:"post" tags:"全局机器人" summary:"解析Telegram自定义Emoji"`
	sysin.CustomEmojiResolveInp
}

type CustomEmojiResolveRes struct {
	List []*sysin.CustomEmojiModel `json:"list" dc:"Emoji资源列表"`
}
