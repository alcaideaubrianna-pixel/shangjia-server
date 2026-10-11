// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishRelayMedia is the golang structure for table youban_publish_relay_media.
type YoubanPublishRelayMedia struct {
	Id             int64       `json:"id"             orm:"id"               ` //
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"        ` //
	MediaId        int64       `json:"mediaId"        orm:"media_id"         ` //
	Purpose        string      `json:"purpose"        orm:"purpose"          ` //
	MediaVersion   string      `json:"mediaVersion"   orm:"media_version"    ` //
	RelayChannelId int64       `json:"relayChannelId" orm:"relay_channel_id" ` //
	RelayChatId    string      `json:"relayChatId"    orm:"relay_chat_id"    ` //
	MessageId      int64       `json:"messageId"      orm:"message_id"       ` //
	FileId         string      `json:"fileId"         orm:"file_id"          ` //
	Status         string      `json:"status"         orm:"status"           ` //
	LeaseToken     string      `json:"leaseToken"     orm:"lease_token"      ` //
	LeaseUntil     *gtime.Time `json:"leaseUntil"     orm:"lease_until"      ` //
	LastError      string      `json:"lastError"      orm:"last_error"       ` //
	ExpiresAt      *gtime.Time `json:"expiresAt"      orm:"expires_at"       ` //
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"       ` //
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"       ` //
}
