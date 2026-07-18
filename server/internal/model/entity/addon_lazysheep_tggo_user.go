// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoUser is the golang structure for table addon_lazysheep_tggo_user.
type AddonLazysheepTggoUser struct {
	Id           int64       `json:"id"           orm:"id"             description:"主键"`
	TelegramId   int64       `json:"telegramId"   orm:"telegram_id"    description:"Telegram 用户ID"`
	BotKey       string      `json:"botKey"       orm:"bot_key"        description:"机器人标识"`
	MemberId     int64       `json:"memberId"     orm:"member_id"      description:"后台用户ID"`
	Username     string      `json:"username"     orm:"username"       description:"用户名"`
	FirstName    string      `json:"firstName"    orm:"first_name"     description:"名"`
	LastName     string      `json:"lastName"     orm:"last_name"      description:"姓"`
	LanguageCode string      `json:"languageCode" orm:"language_code"  description:"语言"`
	IsBot        int         `json:"isBot"        orm:"is_bot"         description:"是否机器人"`
	MemberLevel  int         `json:"memberLevel"  orm:"member_level"   description:"会员等级"`
	Points       float64     `json:"points"       orm:"points"         description:"积分"`
	LastActiveAt *gtime.Time `json:"lastActiveAt" orm:"last_active_at" description:"最后活跃时间"`
	Status       int         `json:"status"       orm:"status"         description:"状态"`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"     description:"创建时间"`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"     description:"更新时间"`
}
