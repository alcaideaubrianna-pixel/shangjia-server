// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMaterialImportTask is the golang structure for table youban_publish_material_import_task.
type YoubanPublishMaterialImportTask struct {
	Id             int64       `json:"id"             orm:"id"              description:""`
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"       description:""`
	AccountId      int64       `json:"accountId"      orm:"account_id"      description:""`
	TgAccountId    int64       `json:"tgAccountId"    orm:"tg_account_id"   description:""`
	SourceChatId   string      `json:"sourceChatId"   orm:"source_chat_id"  description:""`
	SourceTitle    string      `json:"sourceTitle"    orm:"source_title"    description:""`
	SourceUsername string      `json:"sourceUsername" orm:"source_username" description:""`
	Status         string      `json:"status"         orm:"status"          description:""`
	Stage          string      `json:"stage"          orm:"stage"           description:""`
	PullOffsetId   int64       `json:"pullOffsetId"   orm:"pull_offset_id"  description:""`
	PullLimitDays  int         `json:"pullLimitDays"  orm:"pull_limit_days" description:""`
	MessageTotal   int         `json:"messageTotal"   orm:"message_total"   description:""`
	MessageDone    int         `json:"messageDone"    orm:"message_done"    description:""`
	GroupTotal     int         `json:"groupTotal"     orm:"group_total"     description:""`
	GroupDone      int         `json:"groupDone"      orm:"group_done"      description:""`
	MediaTotal     int         `json:"mediaTotal"     orm:"media_total"     description:""`
	MediaDone      int         `json:"mediaDone"      orm:"media_done"      description:""`
	MediaFailed    int         `json:"mediaFailed"    orm:"media_failed"    description:""`
	Imported       int         `json:"imported"       orm:"imported"        description:""`
	Duplicate      int         `json:"duplicate"      orm:"duplicate"       description:""`
	ErrorMessage   string      `json:"errorMessage"   orm:"error_message"   description:""`
	NextRunAt      *gtime.Time `json:"nextRunAt"      orm:"next_run_at"     description:""`
	ResultJson     string      `json:"resultJson"     orm:"result_json"     description:""`
	CreatedBy      int64       `json:"createdBy"      orm:"created_by"      description:""`
	UpdatedBy      int64       `json:"updatedBy"      orm:"updated_by"      description:""`
	StartedAt      *gtime.Time `json:"startedAt"      orm:"started_at"      description:""`
	FinishedAt     *gtime.Time `json:"finishedAt"     orm:"finished_at"     description:""`
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"      description:""`
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"      description:""`
}
