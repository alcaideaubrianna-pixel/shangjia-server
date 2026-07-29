// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectReview is the golang structure for table youban_publish_collect_review.
type YoubanPublishCollectReview struct {
	Id                  uint64      `json:"id"                  orm:"id"                     description:"主键"`
	TenantId            int64       `json:"tenantId"            orm:"tenant_id"              description:"租户ID"`
	AccountId           int64       `json:"accountId"           orm:"account_id"             description:"所属账号ID"`
	SourceId            int64       `json:"sourceId"            orm:"source_id"              description:"采集源ID"`
	RuleId              int64       `json:"ruleId"              orm:"rule_id"                description:"规则ID"`
	EventId             int64       `json:"eventId"             orm:"event_id"               description:"采集事件ID"`
	DispatchId          int64       `json:"dispatchId"          orm:"dispatch_id"            description:"分发记录ID"`
	RawText             string      `json:"rawText"             orm:"raw_text"               description:"原始文本"`
	MediaCount          int         `json:"mediaCount"          orm:"media_count"            description:"媒体数量"`
	TargetChannelIdJson string      `json:"targetChannelIdJson" orm:"target_channel_id_json" description:"目标频道ID JSON"`
	BotIdJson           string      `json:"botIdJson"           orm:"bot_id_json"            description:"推送BOT ID JSON"`
	Status              string      `json:"status"              orm:"status"                 description:"审核状态"`
	ReviewReason        string      `json:"reviewReason"        orm:"review_reason"          description:"审核原因"`
	ReviewedBy          int64       `json:"reviewedBy"          orm:"reviewed_by"            description:"审核人"`
	ReviewedAt          *gtime.Time `json:"reviewedAt"          orm:"reviewed_at"            description:"审核时间"`
	CreatedAt           *gtime.Time `json:"createdAt"           orm:"created_at"             description:"创建时间"`
	UpdatedAt           *gtime.Time `json:"updatedAt"           orm:"updated_at"             description:"更新时间"`
}
