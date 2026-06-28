// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgLogin is the golang structure of table hg_youban_publish_tg_login for DAO operations like Where/Data.
type YoubanPublishTgLogin struct {
	g.Meta           `orm:"table:hg_youban_publish_tg_login, do:true"`
	Id               any         // 主键
	MerchantId       any         // 商家ID
	AccountId        any         // 账号ID
	LoginToken       any         // 登录令牌
	QrUrl            any         // 二维码地址
	TelegramUserId   any         // TG用户ID
	TelegramUsername any         // TG用户名
	SessionKey       any         // 会话存储键
	Status           any         // 状态
	ErrorMessage     any         // 错误信息
	ExpiresAt        *gtime.Time // 过期时间
	CreatedAt        *gtime.Time // 创建时间
	UpdatedAt        *gtime.Time // 更新时间
}
