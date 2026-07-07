// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgMessageCache is the golang structure for table youban_publish_tg_message_cache.
type YoubanPublishTgMessageCache struct {
	Id           int64       `json:"id"           orm:"id"             description:""`
	TenantId     int64       `json:"tenantId"     orm:"tenant_id"      description:""`
	TgAccountId  int64       `json:"tgAccountId"  orm:"tg_account_id"  description:""`
	ChannelId    int64       `json:"channelId"    orm:"channel_id"     description:""`
	TargetChatId string      `json:"targetChatId" orm:"target_chat_id" description:""`
	TgMessageId  int64       `json:"tgMessageId"  orm:"tg_message_id"  description:""`
	MessageText  string      `json:"messageText"  orm:"message_text"   description:""`
	MessageDate  *gtime.Time `json:"messageDate"  orm:"message_date"   description:""`
	MediaGroupId string      `json:"mediaGroupId" orm:"media_group_id" description:""`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"     description:""`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"     description:""`
}
