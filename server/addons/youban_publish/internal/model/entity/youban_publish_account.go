// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishAccount is the golang structure for table youban_publish_account.
type YoubanPublishAccount struct {
	Id                     uint64      `json:"id"                     orm:"id"                       description:"主键"`
	TenantId               int64       `json:"tenantId"               orm:"tenant_id"                description:"租户ID"`
	MerchantId             int64       `json:"merchantId"             orm:"merchant_id"              description:"商家ID"`
	AdminMemberId          int64       `json:"adminMemberId"          orm:"admin_member_id"          description:"绑定系统账号ID"`
	ParentId               int64       `json:"parentId"               orm:"parent_id"                description:"父账号ID"`
	AccountType            string      `json:"accountType"            orm:"account_type"             description:"账号类型"`
	Nickname               string      `json:"nickname"               orm:"nickname"                 description:"昵称"`
	Username               string      `json:"username"               orm:"username"                 description:"用户名"`
	PasswordHash           string      `json:"passwordHash"           orm:"password_hash"            description:"密码hash"`
	Salt                   string      `json:"salt"                   orm:"salt"                     description:"密码盐"`
	TelegramUserId         string      `json:"telegramUserId"         orm:"telegram_user_id"         description:"TG用户ID"`
	TelegramUsername       string      `json:"telegramUsername"       orm:"telegram_username"        description:"TG用户名"`
	DailyPublishLimit      int         `json:"dailyPublishLimit"      orm:"daily_publish_limit"      description:"每日上架额度"`
	CanDirectPublish       int         `json:"canDirectPublish"       orm:"can_direct_publish"       description:"是否可直接发布"`
	AllowedChannelJson     string      `json:"allowedChannelJson"     orm:"allowed_channel_json"     description:"可发布频道JSON"`
	AllowedRegionJson      string      `json:"allowedRegionJson"      orm:"allowed_region_json"      description:"可发布地区JSON"`
	Remark                 string      `json:"remark"                 orm:"remark"                   description:"备注"`
	Status                 int         `json:"status"                 orm:"status"                   description:"状态"`
	CreatedBy              int64       `json:"createdBy"              orm:"created_by"               description:"创建人"`
	UpdatedBy              int64       `json:"updatedBy"              orm:"updated_by"               description:"更新人"`
	DeletedBy              int64       `json:"deletedBy"              orm:"deleted_by"               description:"删除人"`
	CreatedAt              *gtime.Time `json:"createdAt"              orm:"created_at"               description:"创建时间"`
	UpdatedAt              *gtime.Time `json:"updatedAt"              orm:"updated_at"               description:"更新时间"`
	DeletedAt              *gtime.Time `json:"deletedAt"              orm:"deleted_at"               description:"删除时间"`
	AvatarUrl              string      `json:"avatarUrl"              orm:"avatar_url"               description:"头像地址"`
	ContactTelegram        string      `json:"contactTelegram"        orm:"contact_telegram"         description:"联系TG"`
	ContactWechat          string      `json:"contactWechat"          orm:"contact_wechat"           description:"联系微信"`
	ContactPhone           string      `json:"contactPhone"           orm:"contact_phone"            description:"联系电话"`
	ContactOther           string      `json:"contactOther"           orm:"contact_other"            description:"其他联系方式"`
	FollowApprovalRequired int         `json:"followApprovalRequired" orm:"follow_approval_required" description:"关注我是否需要审批"`
	PublicFollowEnabled    int         `json:"publicFollowEnabled"    orm:"public_follow_enabled"    description:"是否公开关注"`
}
