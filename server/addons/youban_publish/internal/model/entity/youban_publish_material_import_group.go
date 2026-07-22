// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMaterialImportGroup is the golang structure for table youban_publish_material_import_group.
type YoubanPublishMaterialImportGroup struct {
	Id               int64       `json:"id"               orm:"id"                 description:""`
	TaskId           int64       `json:"taskId"           orm:"task_id"            description:""`
	TenantId         int64       `json:"tenantId"         orm:"tenant_id"          description:""`
	AccountId        int64       `json:"accountId"        orm:"account_id"         description:""`
	SourceChatId     string      `json:"sourceChatId"     orm:"source_chat_id"     description:""`
	SourceGroupedId  string      `json:"sourceGroupedId"  orm:"source_grouped_id"  description:""`
	SourceMessageIds string      `json:"sourceMessageIds" orm:"source_message_ids" description:""`
	SourceUniqueKey  string      `json:"sourceUniqueKey"  orm:"source_unique_key"  description:""`
	Title            string      `json:"title"            orm:"title"              description:""`
	Nickname         string      `json:"nickname"         orm:"nickname"           description:""`
	ProfileNo        string      `json:"profileNo"        orm:"profile_no"         description:""`
	RawText          string      `json:"rawText"          orm:"raw_text"           description:""`
	ProfileText      string      `json:"profileText"      orm:"profile_text"       description:""`
	VerifyText       string      `json:"verifyText"       orm:"verify_text"        description:""`
	MediaJson        string      `json:"mediaJson"        orm:"media_json"         description:""`
	MediaTotal       int         `json:"mediaTotal"       orm:"media_total"        description:""`
	MediaDone        int         `json:"mediaDone"        orm:"media_done"         description:""`
	MediaFailed      int         `json:"mediaFailed"      orm:"media_failed"       description:""`
	ProfileId        int64       `json:"profileId"        orm:"profile_id"         description:""`
	TaskProfileId    int64       `json:"taskProfileId"    orm:"task_profile_id"    description:""`
	Status           string      `json:"status"           orm:"status"             description:""`
	ErrorMessage     string      `json:"errorMessage"     orm:"error_message"      description:""`
	MessageAt        *gtime.Time `json:"messageAt"        orm:"message_at"         description:""`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"         description:""`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"         description:""`
}
