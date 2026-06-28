// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishAccount is the golang structure for table youban_publish_account.
type YoubanPublishAccount struct {
	Id                 int64       `json:"id"                 orm:"id"                   description:""`
	MerchantId         int64       `json:"merchantId"         orm:"merchant_id"          description:""`
	AdminMemberId      int64       `json:"adminMemberId"      orm:"admin_member_id"      description:""`
	ParentId           int64       `json:"parentId"           orm:"parent_id"            description:""`
	AccountType        string      `json:"accountType"        orm:"account_type"         description:""`
	Nickname           string      `json:"nickname"           orm:"nickname"             description:""`
	Username           string      `json:"username"           orm:"username"             description:""`
	TelegramUserId     string      `json:"telegramUserId"     orm:"telegram_user_id"     description:""`
	TelegramUsername   string      `json:"telegramUsername"   orm:"telegram_username"    description:""`
	DailyPublishLimit  int         `json:"dailyPublishLimit"  orm:"daily_publish_limit"  description:""`
	CanDirectPublish   int         `json:"canDirectPublish"   orm:"can_direct_publish"   description:""`
	AllowedChannelJson string      `json:"allowedChannelJson" orm:"allowed_channel_json" description:""`
	AllowedRegionJson  string      `json:"allowedRegionJson"  orm:"allowed_region_json"  description:""`
	Remark             string      `json:"remark"             orm:"remark"               description:""`
	Status             int         `json:"status"             orm:"status"               description:""`
	CreatedBy          int64       `json:"createdBy"          orm:"created_by"           description:""`
	UpdatedBy          int64       `json:"updatedBy"          orm:"updated_by"           description:""`
	DeletedBy          int64       `json:"deletedBy"          orm:"deleted_by"           description:""`
	CreatedAt          *gtime.Time `json:"createdAt"          orm:"created_at"           description:""`
	UpdatedAt          *gtime.Time `json:"updatedAt"          orm:"updated_at"           description:""`
	DeletedAt          *gtime.Time `json:"deletedAt"          orm:"deleted_at"           description:""`
	TenantId           int64       `json:"tenantId"           orm:"tenant_id"            description:""`
}
