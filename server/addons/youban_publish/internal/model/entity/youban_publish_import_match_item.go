// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishImportMatchItem is the golang structure for table youban_publish_import_match_item.
type YoubanPublishImportMatchItem struct {
	Id              int64       `json:"id"              orm:"id"                ` //
	MatchRunId      int64       `json:"matchRunId"      orm:"match_run_id"      ` //
	ImportRunId     int64       `json:"importRunId"     orm:"import_run_id"     ` //
	TenantId        int64       `json:"tenantId"        orm:"tenant_id"         ` //
	AccountId       int64       `json:"accountId"       orm:"account_id"        ` //
	ProfileId       int64       `json:"profileId"       orm:"profile_id"        ` //
	TaskId          int64       `json:"taskId"          orm:"task_id"           ` //
	ChannelId       int64       `json:"channelId"       orm:"channel_id"        ` //
	DisplayGroupKey string      `json:"displayGroupKey" orm:"display_group_key" ` //
	VerifyGroupKey  string      `json:"verifyGroupKey"  orm:"verify_group_key"  ` //
	DisplayScore    int         `json:"displayScore"    orm:"display_score"     ` //
	VerifyScore     int         `json:"verifyScore"     orm:"verify_score"      ` //
	TotalScore      int         `json:"totalScore"      orm:"total_score"       ` //
	MatchStatus     string      `json:"matchStatus"     orm:"match_status"      ` //
	MatchMode       string      `json:"matchMode"       orm:"match_mode"        ` //
	ReasonJson      string      `json:"reasonJson"      orm:"reason_json"       ` //
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        ` //
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        ` //
	DeletedAt       *gtime.Time `json:"deletedAt"       orm:"deleted_at"        ` //
}
