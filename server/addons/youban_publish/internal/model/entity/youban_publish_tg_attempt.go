// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgAttempt is the golang structure for table youban_publish_tg_attempt.
type YoubanPublishTgAttempt struct {
	Id               int64       `json:"id"               orm:"id"                description:""`
	JobId            int64       `json:"jobId"            orm:"job_id"            description:""`
	TenantId         int64       `json:"tenantId"         orm:"tenant_id"         description:""`
	BotId            int64       `json:"botId"            orm:"bot_id"            description:""`
	ChannelId        int64       `json:"channelId"        orm:"channel_id"        description:""`
	TargetChatId     string      `json:"targetChatId"     orm:"target_chat_id"    description:""`
	Phase            string      `json:"phase"            orm:"phase"             description:""`
	ChunkIndex       int         `json:"chunkIndex"       orm:"chunk_index"       description:""`
	AttemptNo        int         `json:"attemptNo"        orm:"attempt_no"        description:""`
	AttemptToken     string      `json:"attemptToken"     orm:"attempt_token"     description:""`
	ExpectedCount    int         `json:"expectedCount"    orm:"expected_count"    description:""`
	ConfirmedCount   int         `json:"confirmedCount"   orm:"confirmed_count"   description:""`
	Status           string      `json:"status"           orm:"status"            description:""`
	ResponseReceived int         `json:"responseReceived" orm:"response_received" description:""`
	WebhookDeadline  *gtime.Time `json:"webhookDeadline"  orm:"webhook_deadline"  description:""`
	ErrorMessage     string      `json:"errorMessage"     orm:"error_message"     description:""`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"        description:""`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"        description:""`
}
