// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotManagedMember is the golang structure for table youban_bot_managed_member.
type YoubanBotManagedMember struct {
	Id           int64       `json:"id"           orm:"id"             ` //
	TenantId     int64       `json:"tenantId"     orm:"tenant_id"      ` //
	AccountId    int64       `json:"accountId"    orm:"account_id"     ` //
	ManagedBotId int64       `json:"managedBotId" orm:"managed_bot_id" ` //
	ChannelId    int64       `json:"channelId"    orm:"channel_id"     ` //
	MemberStatus string      `json:"memberStatus" orm:"member_status"  ` //
	RightsJson   string      `json:"rightsJson"   orm:"rights_json"    ` //
	LastSyncedAt *gtime.Time `json:"lastSyncedAt" orm:"last_synced_at" ` //
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"     ` //
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"     ` //
}
