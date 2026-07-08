// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgChannelStat is the golang structure for table youban_publish_tg_channel_stat.
type YoubanPublishTgChannelStat struct {
	Id               int64       `json:"id"               orm:"id"                 description:""`
	TenantId         int64       `json:"tenantId"         orm:"tenant_id"          description:""`
	AccountId        int64       `json:"accountId"        orm:"account_id"         description:""`
	ChannelId        int64       `json:"channelId"        orm:"channel_id"         description:""`
	TargetChatId     string      `json:"targetChatId"     orm:"target_chat_id"     description:""`
	ChannelTitle     string      `json:"channelTitle"     orm:"channel_title"      description:""`
	PendingCount     int         `json:"pendingCount"     orm:"pending_count"      description:""`
	QueuedCount      int         `json:"queuedCount"      orm:"queued_count"       description:""`
	SendingCount     int         `json:"sendingCount"     orm:"sending_count"      description:""`
	SentCount        int         `json:"sentCount"        orm:"sent_count"         description:""`
	FailedCount      int         `json:"failedCount"      orm:"failed_count"       description:""`
	RetryCount       int         `json:"retryCount"       orm:"retry_count"        description:""`
	RateLimitCount   int         `json:"rateLimitCount"   orm:"rate_limit_count"   description:""`
	LastSentAt       *gtime.Time `json:"lastSentAt"       orm:"last_sent_at"       description:""`
	LastErrorAt      *gtime.Time `json:"lastErrorAt"      orm:"last_error_at"      description:""`
	LastErrorMessage string      `json:"lastErrorMessage" orm:"last_error_message" description:""`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"         description:""`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"         description:""`
}
