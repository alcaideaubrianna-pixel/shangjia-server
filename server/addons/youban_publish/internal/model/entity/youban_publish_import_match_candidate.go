// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishImportMatchCandidate is the golang structure for table youban_publish_import_match_candidate.
type YoubanPublishImportMatchCandidate struct {
	Id             int64       `json:"id"             orm:"id"               ` //
	MatchRunId     int64       `json:"matchRunId"     orm:"match_run_id"     ` //
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"        ` //
	ChannelId      int64       `json:"channelId"      orm:"channel_id"       ` //
	GroupKey       string      `json:"groupKey"       orm:"group_key"        ` //
	MediaGroupId   string      `json:"mediaGroupId"   orm:"media_group_id"   ` //
	FirstMessageId int64       `json:"firstMessageId" orm:"first_message_id" ` //
	LastMessageId  int64       `json:"lastMessageId"  orm:"last_message_id"  ` //
	MessageDate    *gtime.Time `json:"messageDate"    orm:"message_date"     ` //
	CaptionText    string      `json:"captionText"    orm:"caption_text"     ` //
	MediaCount     int         `json:"mediaCount"     orm:"media_count"      ` //
	MediaTypes     string      `json:"mediaTypes"     orm:"media_types"      ` //
	PreviewJson    string      `json:"previewJson"    orm:"preview_json"     ` //
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"       ` //
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"       ` //
	DeletedAt      *gtime.Time `json:"deletedAt"      orm:"deleted_at"       ` //
}
