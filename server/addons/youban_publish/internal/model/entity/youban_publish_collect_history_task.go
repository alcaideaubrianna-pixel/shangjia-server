// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectHistoryTask is the golang structure for table youban_publish_collect_history_task.
type YoubanPublishCollectHistoryTask struct {
	Id             int64       `json:"id"             orm:"id"              description:""`
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"       description:""`
	AccountId      int64       `json:"accountId"      orm:"account_id"      description:""`
	SourceId       int64       `json:"sourceId"       orm:"source_id"       description:""`
	TgAccountId    int64       `json:"tgAccountId"    orm:"tg_account_id"   description:""`
	SourceChatId   string      `json:"sourceChatId"   orm:"source_chat_id"  description:""`
	Mode           string      `json:"mode"           orm:"mode"            description:""`
	Days           int         `json:"days"           orm:"days"            description:""`
	OffsetId       int         `json:"offsetId"       orm:"offset_id"       description:""`
	ScannedCount   int         `json:"scannedCount"   orm:"scanned_count"   description:""`
	EventCount     int         `json:"eventCount"     orm:"event_count"     description:""`
	DuplicateCount int         `json:"duplicateCount" orm:"duplicate_count" description:""`
	FailedCount    int         `json:"failedCount"    orm:"failed_count"    description:""`
	Status         string      `json:"status"         orm:"status"          description:""`
	ErrorMessage   string      `json:"errorMessage"   orm:"error_message"   description:""`
	NextRunAt      *gtime.Time `json:"nextRunAt"      orm:"next_run_at"     description:""`
	StartedAt      *gtime.Time `json:"startedAt"      orm:"started_at"      description:""`
	FinishedAt     *gtime.Time `json:"finishedAt"     orm:"finished_at"     description:""`
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"      description:""`
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"      description:""`
}
