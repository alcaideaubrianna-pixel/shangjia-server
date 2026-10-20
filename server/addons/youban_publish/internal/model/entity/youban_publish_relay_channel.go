// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishRelayChannel is the golang structure for table youban_publish_relay_channel.
type YoubanPublishRelayChannel struct {
	Id                  int64       `json:"id"                  orm:"id"                    ` //
	TenantId            int64       `json:"tenantId"            orm:"tenant_id"             ` //
	TgAccountId         int64       `json:"tgAccountId"         orm:"tg_account_id"         ` //
	ChannelId           string      `json:"channelId"           orm:"channel_id"            ` //
	TargetChatId        string      `json:"targetChatId"        orm:"target_chat_id"        ` //
	Title               string      `json:"title"               orm:"title"                 ` //
	Status              string      `json:"status"              orm:"status"                ` //
	HealthStatus        string      `json:"healthStatus"        orm:"health_status"         ` //
	BotPermissionStatus string      `json:"botPermissionStatus" orm:"bot_permission_status" ` //
	CooldownUntil       *gtime.Time `json:"cooldownUntil"       orm:"cooldown_until"        ` //
	LastError           string      `json:"lastError"           orm:"last_error"            ` //
	InFlightCount       int         `json:"inFlightCount"       orm:"in_flight_count"       ` //
	LastUsedAt          *gtime.Time `json:"lastUsedAt"          orm:"last_used_at"          ` //
	CreatedAt           *gtime.Time `json:"createdAt"           orm:"created_at"            ` //
	UpdatedAt           *gtime.Time `json:"updatedAt"           orm:"updated_at"            ` //
	DeletedAt           *gtime.Time `json:"deletedAt"           orm:"deleted_at"            ` //
	AccessHash          string      `json:"accessHash"          orm:"access_hash"           ` //
	BotId               int64       `json:"botId"               orm:"bot_id"                ` //
	Capacity            int         `json:"capacity"            orm:"capacity"              ` //
	OperationKind       string      `json:"operationKind"       orm:"operation_kind"        ` //
	OperationStatus     string      `json:"operationStatus"     orm:"operation_status"      ` //
	TaskId              int64       `json:"taskId"              orm:"task_id"               ` //
}
