// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectContent is the golang structure for table youban_publish_collect_content.
type YoubanPublishCollectContent struct {
	Id             int64       `json:"id"             orm:"id"               description:""`
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"        description:""`
	AccountId      int64       `json:"accountId"      orm:"account_id"       description:""`
	FirstEventId   int64       `json:"firstEventId"   orm:"first_event_id"   description:""`
	LastEventId    int64       `json:"lastEventId"    orm:"last_event_id"    description:""`
	SourceType     string      `json:"sourceType"     orm:"source_type"      description:""`
	RawText        string      `json:"rawText"        orm:"raw_text"         description:""`
	NormalizedText string      `json:"normalizedText" orm:"normalized_text"  description:""`
	MediaCount     int         `json:"mediaCount"     orm:"media_count"      description:""`
	MediaSignature string      `json:"mediaSignature" orm:"media_signature"  description:""`
	MediaJson      string      `json:"mediaJson"      orm:"media_json"       description:""`
	TextHash       string      `json:"textHash"       orm:"text_hash"        description:""`
	DedupeKey      string      `json:"dedupeKey"      orm:"dedupe_key"       description:""`
	DuplicateTotal int         `json:"duplicateTotal" orm:"duplicate_total"  description:""`
	Status         string      `json:"status"         orm:"status"           description:""`
	FirstSeenAt    *gtime.Time `json:"firstSeenAt"    orm:"first_seen_at"    description:""`
	PreviousSeenAt *gtime.Time `json:"previousSeenAt" orm:"previous_seen_at" description:""`
	LastSeenAt     *gtime.Time `json:"lastSeenAt"     orm:"last_seen_at"     description:""`
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"       description:""`
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"       description:""`
}
