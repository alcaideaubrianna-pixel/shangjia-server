package publish

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type AccountLoginLinkReq struct {
	g.Meta `path:"/publish/account/loginLink" method:"post" tags:"上架插件后台" summary:"生成账号登录凭证（仅系统超级管理员）"`
	sysin.AccountLoginLinkInp
}

type AccountLoginLinkRes struct {
	*sysin.AccountLoginModel
}
