// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectBotChannel is the golang structure for table youban_publish_collect_bot_channel.
type YoubanPublishCollectBotChannel struct {
	Id            int64       `json:"id"            orm:"id"             description:""`
	TenantId      int64       `json:"tenantId"      orm:"tenant_id"      description:""`
	BotId         int64       `json:"botId"         orm:"bot_id"         description:""`
	ChannelId     int64       `json:"channelId"     orm:"channel_id"     description:""`
	Status        string      `json:"status"        orm:"status"         description:""`
	BotRole       string      `json:"botRole"       orm:"bot_role"       description:""`
	FirstSeenAt   *gtime.Time `json:"firstSeenAt"   orm:"first_seen_at"  description:""`
	LastSeenAt    *gtime.Time `json:"lastSeenAt"    orm:"last_seen_at"   description:""`
	RemovedAt     *gtime.Time `json:"removedAt"     orm:"removed_at"     description:""`
	RemovedReason string      `json:"removedReason" orm:"removed_reason" description:""`
	LastUpdateId  int64       `json:"lastUpdateId"  orm:"last_update_id" description:""`
	CreatedAt     *gtime.Time `json:"createdAt"     orm:"created_at"     description:""`
	UpdatedAt     *gtime.Time `json:"updatedAt"     orm:"updated_at"     description:""`
	DeletedAt     *gtime.Time `json:"deletedAt"     orm:"deleted_at"     description:""`
}
