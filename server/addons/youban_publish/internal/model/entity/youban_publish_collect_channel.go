// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectChannel is the golang structure for table youban_publish_collect_channel.
type YoubanPublishCollectChannel struct {
	Id             int64       `json:"id"             orm:"id"               description:""`
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"        description:""`
	TelegramChatId string      `json:"telegramChatId" orm:"telegram_chat_id" description:""`
	ChatType       string      `json:"chatType"       orm:"chat_type"        description:""`
	Title          string      `json:"title"          orm:"title"            description:""`
	Username       string      `json:"username"       orm:"username"         description:""`
	InviteLink     string      `json:"inviteLink"     orm:"invite_link"      description:""`
	About          string      `json:"about"          orm:"about"            description:""`
	PhotoFileId    string      `json:"photoFileId"    orm:"photo_file_id"    description:""`
	IsBroadcast    int         `json:"isBroadcast"    orm:"is_broadcast"     description:""`
	IsMegagroup    int         `json:"isMegagroup"    orm:"is_megagroup"     description:""`
	LastMessageId  int64       `json:"lastMessageId"  orm:"last_message_id"  description:""`
	LastMessageAt  *gtime.Time `json:"lastMessageAt"  orm:"last_message_at"  description:""`
	LastSeenAt     *gtime.Time `json:"lastSeenAt"     orm:"last_seen_at"     description:""`
	Status         string      `json:"status"         orm:"status"           description:""`
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"       description:""`
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"       description:""`
	DeletedAt      *gtime.Time `json:"deletedAt"      orm:"deleted_at"       description:""`
}
