// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoUser is the golang structure of table hg_addon_lazysheep_tggo_user for DAO operations like Where/Data.
type AddonLazysheepTggoUser struct {
	g.Meta       `orm:"table:hg_addon_lazysheep_tggo_user, do:true"`
	Id           any         // 主键
	TelegramId   any         // Telegram 用户ID
	BotKey       any         // 机器人标识
	MemberId     any         // 后台用户ID
	Username     any         // 用户名
	FirstName    any         // 名
	LastName     any         // 姓
	LanguageCode any         // 语言
	IsBot        any         // 是否机器人
	MemberLevel  any         // 会员等级
	Points       any         // 积分
	LastActiveAt *gtime.Time // 最后活跃时间
	Status       any         // 状态
	CreatedAt    *gtime.Time // 创建时间
	UpdatedAt    *gtime.Time // 更新时间
}
