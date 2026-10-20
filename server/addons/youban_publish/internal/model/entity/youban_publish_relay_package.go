// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishRelayPackage is the golang structure for table youban_publish_relay_package.
type YoubanPublishRelayPackage struct {
	Id             int64       `json:"id"             orm:"id"               ` //
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"        ` //
	JobId          int64       `json:"jobId"          orm:"job_id"           ` //
	BotId          int64       `json:"botId"          orm:"bot_id"           ` //
	TelegramUserId int64       `json:"telegramUserId" orm:"telegram_user_id" ` //
	RelayChannelId int64       `json:"relayChannelId" orm:"relay_channel_id" ` //
	Generation     int         `json:"generation"     orm:"generation"       ` //
	Status         string      `json:"status"         orm:"status"           ` //
	Payload        string      `json:"payload"        orm:"payload"          ` //
	Messages       string      `json:"messages"       orm:"messages"         ` //
	AttemptId      int64       `json:"attemptId"      orm:"attempt_id"       ` //
	TaskId         int64       `json:"taskId"         orm:"task_id"          ` //
	LeaseUntil     *gtime.Time `json:"leaseUntil"     orm:"lease_until"      ` //
	NextRetryAt    *gtime.Time `json:"nextRetryAt"    orm:"next_retry_at"    ` //
	LastError      string      `json:"lastError"      orm:"last_error"       ` //
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"       ` //
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"       ` //
}
