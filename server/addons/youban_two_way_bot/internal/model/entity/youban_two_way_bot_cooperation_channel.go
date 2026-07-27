// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotCooperationChannel is the golang structure for table youban_two_way_bot_cooperation_channel.
type YoubanTwoWayBotCooperationChannel struct {
	Id        int64       `json:"id"        orm:"id"         description:""`
	TenantId  int64       `json:"tenantId"  orm:"tenant_id"  description:""`
	ConfigId  int64       `json:"configId"  orm:"config_id"  description:""`
	ChannelId int64       `json:"channelId" orm:"channel_id" description:""`
	Status    int         `json:"status"    orm:"status"     description:""`
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" description:""`
	UpdatedAt *gtime.Time `json:"updatedAt" orm:"updated_at" description:""`
	DeletedAt *gtime.Time `json:"deletedAt" orm:"deleted_at" description:""`
}
