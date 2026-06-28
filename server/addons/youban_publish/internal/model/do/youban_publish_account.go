// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishAccount is the golang structure of table hg_youban_publish_account for DAO operations like Where/Data.
type YoubanPublishAccount struct {
	g.Meta             `orm:"table:hg_youban_publish_account, do:true"`
	Id                 any         //
	MerchantId         any         //
	AdminMemberId      any         //
	ParentId           any         //
	AccountType        any         //
	Nickname           any         //
	Username           any         //
	TelegramUserId     any         //
	TelegramUsername   any         //
	DailyPublishLimit  any         //
	CanDirectPublish   any         //
	AllowedChannelJson any         //
	AllowedRegionJson  any         //
	Remark             any         //
	Status             any         //
	CreatedBy          any         //
	UpdatedBy          any         //
	DeletedBy          any         //
	CreatedAt          *gtime.Time //
	UpdatedAt          *gtime.Time //
	DeletedAt          *gtime.Time //
	TenantId           any         //
}
