// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotCooperationConfig is the golang structure for table youban_two_way_bot_cooperation_config.
type YoubanTwoWayBotCooperationConfig struct {
	Id               int64       `json:"id"               orm:"id"                description:""`
	TenantId         int64       `json:"tenantId"         orm:"tenant_id"         description:""`
	AccountId        int64       `json:"accountId"        orm:"account_id"        description:""`
	BotId            int64       `json:"botId"            orm:"bot_id"            description:""`
	TwoWayBotId      int64       `json:"twoWayBotId"      orm:"two_way_bot_id"    description:""`
	NotificationType string      `json:"notificationType" orm:"notification_type" description:""`
	ReviewRequired   int         `json:"reviewRequired"   orm:"review_required"   description:""`
	Status           int         `json:"status"           orm:"status"            description:""`
	CreatedBy        int64       `json:"createdBy"        orm:"created_by"        description:""`
	UpdatedBy        int64       `json:"updatedBy"        orm:"updated_by"        description:""`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"        description:""`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"        description:""`
	DeletedAt        *gtime.Time `json:"deletedAt"        orm:"deleted_at"        description:""`
}
