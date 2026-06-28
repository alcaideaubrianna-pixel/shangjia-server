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
	Id               any         //
	MerchantId       any         //
	AccountId        any         //
	LoginToken       any         //
	QrUrl            any         //
	TelegramUserId   any         //
	TelegramUsername any         //
	SessionKey       any         //
	Status           any         //
	ErrorMessage     any         //
	ExpiresAt        *gtime.Time //
	CreatedAt        *gtime.Time //
	UpdatedAt        *gtime.Time //
	TenantId         any         //
}
