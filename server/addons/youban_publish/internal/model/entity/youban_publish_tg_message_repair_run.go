// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgMessageRepairRun is the golang structure for table youban_publish_tg_message_repair_run.
type YoubanPublishTgMessageRepairRun struct {
	Id           int64       `json:"id"           orm:"id"            description:""`
	TenantId     int64       `json:"tenantId"     orm:"tenant_id"     description:""`
	AccountId    int64       `json:"accountId"    orm:"account_id"    description:""`
	ProfileId    int64       `json:"profileId"    orm:"profile_id"    description:""`
	TaskId       int64       `json:"taskId"       orm:"task_id"       description:""`
	Status       string      `json:"status"       orm:"status"        description:""`
	Stage        string      `json:"stage"        orm:"stage"         description:""`
	Progress     int         `json:"progress"     orm:"progress"      description:""`
	ChannelCount int         `json:"channelCount" orm:"channel_count" description:""`
	ScannedCount int         `json:"scannedCount" orm:"scanned_count" description:""`
	MatchedCount int         `json:"matchedCount" orm:"matched_count" description:""`
	ErrorMessage string      `json:"errorMessage" orm:"error_message" description:""`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"    description:""`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"    description:""`
	FinishedAt   *gtime.Time `json:"finishedAt"   orm:"finished_at"   description:""`
}
