// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgJob is the golang structure for table youban_publish_tg_job.
type YoubanPublishTgJob struct {
	Id                     int64       `json:"id"                     orm:"id"                        description:""`
	TaskId                 int64       `json:"taskId"                 orm:"task_id"                   description:""`
	OperationNo            string      `json:"operationNo"            orm:"operation_no"              description:""`
	TenantId               int64       `json:"tenantId"               orm:"tenant_id"                 description:""`
	MerchantId             int64       `json:"merchantId"             orm:"merchant_id"               description:""`
	AccountId              int64       `json:"accountId"              orm:"account_id"                description:""`
	ProfileId              int64       `json:"profileId"              orm:"profile_id"                description:""`
	ChannelId              int64       `json:"channelId"              orm:"channel_id"                description:""`
	BotId                  int64       `json:"botId"                  orm:"bot_id"                    description:""`
	TargetChatId           string      `json:"targetChatId"           orm:"target_chat_id"            description:""`
	CollectEventId         int64       `json:"collectEventId"         orm:"collect_event_id"          description:""`
	CollectSourceId        int64       `json:"collectSourceId"        orm:"collect_source_id"         description:""`
	CollectSourceChatId    string      `json:"collectSourceChatId"    orm:"collect_source_chat_id"    description:""`
	CollectSourceMessageId int64       `json:"collectSourceMessageId" orm:"collect_source_message_id" description:""`
	TgMessageId            int64       `json:"tgMessageId"            orm:"tg_message_id"             description:""`
	AsynqTaskId            string      `json:"asynqTaskId"            orm:"asynq_task_id"             description:""`
	Status                 string      `json:"status"                 orm:"status"                    description:""`
	RetryCount             int         `json:"retryCount"             orm:"retry_count"               description:""`
	NextRetryAt            *gtime.Time `json:"nextRetryAt"            orm:"next_retry_at"             description:""`
	SentAt                 *gtime.Time `json:"sentAt"                 orm:"sent_at"                   description:""`
	CycleEnabled           int         `json:"cycleEnabled"           orm:"cycle_enabled"             description:""`
	CycleDays              int         `json:"cycleDays"              orm:"cycle_days"                description:""`
	CyclePublishTime       string      `json:"cyclePublishTime"       orm:"cycle_publish_time"        description:""`
	NextCycleAt            *gtime.Time `json:"nextCycleAt"            orm:"next_cycle_at"             description:""`
	Priority               int         `json:"priority"               orm:"priority"                  description:""`
	QueueName              string      `json:"queueName"              orm:"queue_name"                description:""`
	DispatchStatus         string      `json:"dispatchStatus"         orm:"dispatch_status"           description:""`
	DispatchedAt           *gtime.Time `json:"dispatchedAt"           orm:"dispatched_at"             description:""`
	DispatchCount          int         `json:"dispatchCount"          orm:"dispatch_count"            description:""`
	SendPhase              string      `json:"sendPhase"              orm:"send_phase"                description:""`
	ReconcileCount         int         `json:"reconcileCount"         orm:"reconcile_count"           description:""`
	LastDispatchError      string      `json:"lastDispatchError"      orm:"last_dispatch_error"       description:""`
	ErrorMessage           string      `json:"errorMessage"           orm:"error_message"             description:""`
	CreatedAt              *gtime.Time `json:"createdAt"              orm:"created_at"                description:""`
	UpdatedAt              *gtime.Time `json:"updatedAt"              orm:"updated_at"                description:""`
	PushMode               string      `json:"pushMode"               orm:"push_mode"                 description:""`
	OperationType          string      `json:"operationType"          orm:"operation_type"            description:""`
	ProfileNoSnapshot      string      `json:"profileNoSnapshot"      orm:"profile_no_snapshot"       description:""`
	TitleSnapshot          string      `json:"titleSnapshot"          orm:"title_snapshot"            description:""`
	NotifyAccountId        int64       `json:"notifyAccountId"        orm:"notify_account_id"         description:""`
	CycleFailureCount      int         `json:"cycleFailureCount"      orm:"cycle_failure_count"       description:""`
	SendDeadlineAt         *gtime.Time `json:"sendDeadlineAt"         orm:"send_deadline_at"          description:""`
	TimeoutParentJobId     int64       `json:"timeoutParentJobId"     orm:"timeout_parent_job_id"     description:""`
	TimeoutCleanupAt       *gtime.Time `json:"timeoutCleanupAt"       orm:"timeout_cleanup_at"        description:""`
	TimeoutRetryCount      int         `json:"timeoutRetryCount"      orm:"timeout_retry_count"       description:""`
	DisplayMediaSelection  string      `json:"displayMediaSelection"  orm:"display_media_selection"   description:""`
}
