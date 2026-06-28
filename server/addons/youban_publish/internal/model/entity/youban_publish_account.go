// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishAccount is the golang structure for table youban_publish_account.
type YoubanPublishAccount struct {
	Id                 uint64      `json:"id"                 orm:"id"                   description:"主键"`
	MerchantId         int64       `json:"merchantId"         orm:"merchant_id"          description:"商家ID"`
	AdminMemberId      int64       `json:"adminMemberId"      orm:"admin_member_id"      description:"绑定系统账号ID"`
	ParentId           int64       `json:"parentId"           orm:"parent_id"            description:"父账号ID"`
	AccountType        string      `json:"accountType"        orm:"account_type"         description:"账号类型"`
	Nickname           string      `json:"nickname"           orm:"nickname"             description:"昵称"`
	Username           string      `json:"username"           orm:"username"             description:"用户名"`
	TelegramUserId     string      `json:"telegramUserId"     orm:"telegram_user_id"     description:"TG用户ID"`
	TelegramUsername   string      `json:"telegramUsername"   orm:"telegram_username"    description:"TG用户名"`
	DailyPublishLimit  int         `json:"dailyPublishLimit"  orm:"daily_publish_limit"  description:"每日上架额度"`
	CanDirectPublish   int         `json:"canDirectPublish"   orm:"can_direct_publish"   description:"是否可直接发布"`
	AllowedChannelJson string      `json:"allowedChannelJson" orm:"allowed_channel_json" description:"可发布频道JSON"`
	AllowedRegionJson  string      `json:"allowedRegionJson"  orm:"allowed_region_json"  description:"可发布地区JSON"`
	Remark             string      `json:"remark"             orm:"remark"               description:"备注"`
	Status             int         `json:"status"             orm:"status"               description:"状态"`
	CreatedBy          int64       `json:"createdBy"          orm:"created_by"           description:"创建人"`
	UpdatedBy          int64       `json:"updatedBy"          orm:"updated_by"           description:"更新人"`
	DeletedBy          int64       `json:"deletedBy"          orm:"deleted_by"           description:"删除人"`
	CreatedAt          *gtime.Time `json:"createdAt"          orm:"created_at"           description:"创建时间"`
	UpdatedAt          *gtime.Time `json:"updatedAt"          orm:"updated_at"           description:"更新时间"`
	DeletedAt          *gtime.Time `json:"deletedAt"          orm:"deleted_at"           description:"删除时间"`
}
