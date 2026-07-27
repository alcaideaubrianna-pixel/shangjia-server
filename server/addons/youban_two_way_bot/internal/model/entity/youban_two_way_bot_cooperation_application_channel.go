// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotCooperationApplicationChannel is the golang structure for table youban_two_way_bot_cooperation_application_channel.
type YoubanTwoWayBotCooperationApplicationChannel struct {
	Id            int64       `json:"id"            orm:"id"             description:""`
	TenantId      int64       `json:"tenantId"      orm:"tenant_id"      description:""`
	ApplicationId int64       `json:"applicationId" orm:"application_id" description:""`
	ChannelId     int64       `json:"channelId"     orm:"channel_id"     description:""`
	Status        string      `json:"status"        orm:"status"         description:""`
	ErrorMessage  string      `json:"errorMessage"  orm:"error_message"  description:""`
	RetryCount    int         `json:"retryCount"    orm:"retry_count"    description:""`
	JoinedAt      *gtime.Time `json:"joinedAt"      orm:"joined_at"      description:""`
	CreatedAt     *gtime.Time `json:"createdAt"     orm:"created_at"     description:""`
	UpdatedAt     *gtime.Time `json:"updatedAt"     orm:"updated_at"     description:""`
}
