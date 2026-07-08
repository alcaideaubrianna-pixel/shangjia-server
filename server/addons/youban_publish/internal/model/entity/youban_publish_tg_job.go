// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgJob is the golang structure for table youban_publish_tg_job.
type YoubanPublishTgJob struct {
	Id               int64       `json:"id"               orm:"id"                 description:""`
	TaskId           int64       `json:"taskId"           orm:"task_id"            description:""`
	TenantId         int64       `json:"tenantId"         orm:"tenant_id"          description:""`
	MerchantId       int64       `json:"merchantId"       orm:"merchant_id"        description:""`
	AccountId        int64       `json:"accountId"        orm:"account_id"         description:""`
	ProfileId        int64       `json:"profileId"        orm:"profile_id"         description:""`
	BotId            int64       `json:"botId"            orm:"bot_id"             description:""`
	TargetChatId     string      `json:"targetChatId"     orm:"target_chat_id"     description:""`
	TgMessageId      int64       `json:"tgMessageId"      orm:"tg_message_id"      description:""`
	Status           string      `json:"status"           orm:"status"             description:""`
	RetryCount       int         `json:"retryCount"       orm:"retry_count"        description:""`
	NextRetryAt      *gtime.Time `json:"nextRetryAt"      orm:"next_retry_at"      description:""`
	ErrorMessage     string      `json:"errorMessage"     orm:"error_message"      description:""`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"         description:""`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"         description:""`
	ChannelId        int64       `json:"channelId"        orm:"channel_id"         description:""`
	SentAt           *gtime.Time `json:"sentAt"           orm:"sent_at"            description:""`
	CycleEnabled     int         `json:"cycleEnabled"     orm:"cycle_enabled"      description:""`
	CycleDays        int         `json:"cycleDays"        orm:"cycle_days"         description:""`
	NextCycleAt      *gtime.Time `json:"nextCycleAt"      orm:"next_cycle_at"      description:""`
	CyclePublishTime string      `json:"cyclePublishTime" orm:"cycle_publish_time" description:""`
	AsynqTaskId      string      `json:"asynqTaskId"      orm:"asynq_task_id"      description:""`
	OperationNo      string      `json:"operationNo"      orm:"operation_no"       description:""`
}
