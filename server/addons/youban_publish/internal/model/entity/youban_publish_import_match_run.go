// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishImportMatchRun is the golang structure for table youban_publish_import_match_run.
type YoubanPublishImportMatchRun struct {
	Id             int64       `json:"id"             orm:"id"              ` //
	ImportRunId    int64       `json:"importRunId"    orm:"import_run_id"   ` //
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"       ` //
	AccountId      int64       `json:"accountId"      orm:"account_id"      ` //
	Status         string      `json:"status"         orm:"status"          ` //
	Stage          string      `json:"stage"          orm:"stage"           ` //
	ChannelIdJson  string      `json:"channelIdJson"  orm:"channel_id_json" ` //
	ScanDays       int         `json:"scanDays"       orm:"scan_days"       ` //
	Threshold      int         `json:"threshold"      orm:"threshold"       ` //
	ProfileTotal   int         `json:"profileTotal"   orm:"profile_total"   ` //
	ProfileDone    int         `json:"profileDone"    orm:"profile_done"    ` //
	CandidateTotal int         `json:"candidateTotal" orm:"candidate_total" ` //
	AutoMatched    int         `json:"autoMatched"    orm:"auto_matched"    ` //
	ManualPending  int         `json:"manualPending"  orm:"manual_pending"  ` //
	Confirmed      int         `json:"confirmed"      orm:"confirmed"       ` //
	Skipped        int         `json:"skipped"        orm:"skipped"         ` //
	ErrorMessage   string      `json:"errorMessage"   orm:"error_message"   ` //
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"      ` //
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"      ` //
	FinishedAt     *gtime.Time `json:"finishedAt"     orm:"finished_at"     ` //
	DeletedAt      *gtime.Time `json:"deletedAt"      orm:"deleted_at"      ` //
}
