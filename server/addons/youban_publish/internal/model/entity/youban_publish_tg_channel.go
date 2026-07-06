// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgChannel is the golang structure for table youban_publish_tg_channel.
type YoubanPublishTgChannel struct {
	Id              uint64      `json:"id"              orm:"id"                description:"主键"`
	TenantId        int64       `json:"tenantId"        orm:"tenant_id"         description:"租户ID"`
	MerchantId      int64       `json:"merchantId"      orm:"merchant_id"       description:"兼容旧版本商家ID"`
	TgAccountId     int64       `json:"tgAccountId"     orm:"tg_account_id"     description:"TG账号ID"`
	ChannelId       string      `json:"channelId"       orm:"channel_id"        description:"频道ID"`
	AccessHash      string      `json:"accessHash"      orm:"access_hash"       description:"AccessHash"`
	ChannelTitle    string      `json:"channelTitle"    orm:"channel_title"     description:"频道名称"`
	ChannelUsername string      `json:"channelUsername" orm:"channel_username"  description:"频道用户名"`
	IsBroadcast     int         `json:"isBroadcast"     orm:"is_broadcast"      description:"是否频道"`
	IsMegagroup     int         `json:"isMegagroup"     orm:"is_megagroup"      description:"是否群组"`
	CanPostMessages int         `json:"canPostMessages" orm:"can_post_messages" description:"账号可发频道消息"`
	CanInviteUsers  int         `json:"canInviteUsers"  orm:"can_invite_users"  description:"账号可邀请用户"`
	CanAddAdmins    int         `json:"canAddAdmins"    orm:"can_add_admins"    description:"账号可添加管理员"`
	LastSyncAt      *gtime.Time `json:"lastSyncAt"      orm:"last_sync_at"      description:"最后同步时间"`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        description:"更新时间"`
}
