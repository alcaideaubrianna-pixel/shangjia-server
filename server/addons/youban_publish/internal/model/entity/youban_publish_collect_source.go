// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectSource is the golang structure for table youban_publish_collect_source.
type YoubanPublishCollectSource struct {
	Id                 int64       `json:"id"                 orm:"id"                  description:""`
	TenantId           int64       `json:"tenantId"           orm:"tenant_id"           description:""`
	AccountId          int64       `json:"accountId"          orm:"account_id"          description:""`
	SourceType         string      `json:"sourceType"         orm:"source_type"         description:""`
	Title              string      `json:"title"              orm:"title"               description:""`
	SourceChatId       string      `json:"sourceChatId"       orm:"source_chat_id"      description:""`
	SourceUsername     string      `json:"sourceUsername"     orm:"source_username"     description:""`
	TgAccountId        int64       `json:"tgAccountId"        orm:"tg_account_id"       description:""`
	BotId              int64       `json:"botId"              orm:"bot_id"              description:""`
	BotCollectScope    string      `json:"botCollectScope"    orm:"bot_collect_scope"   description:""`
	FollowAccountId    int64       `json:"followAccountId"    orm:"follow_account_id"   description:""`
	CollectEnabled     int         `json:"collectEnabled"     orm:"collect_enabled"     description:""`
	Status             int         `json:"status"             orm:"status"              description:""`
	EventTotal         int64       `json:"eventTotal"         orm:"event_total"         description:""`
	SuccessTotal       int64       `json:"successTotal"       orm:"success_total"       description:""`
	FailedTotal        int64       `json:"failedTotal"        orm:"failed_total"        description:""`
	LastEventAt        *gtime.Time `json:"lastEventAt"        orm:"last_event_at"       description:""`
	Remark             string      `json:"remark"             orm:"remark"              description:""`
	CreatedBy          int64       `json:"createdBy"          orm:"created_by"          description:""`
	UpdatedBy          int64       `json:"updatedBy"          orm:"updated_by"          description:""`
	DeletedBy          int64       `json:"deletedBy"          orm:"deleted_by"          description:""`
	CreatedAt          *gtime.Time `json:"createdAt"          orm:"created_at"          description:""`
	UpdatedAt          *gtime.Time `json:"updatedAt"          orm:"updated_at"          description:""`
	DeletedAt          *gtime.Time `json:"deletedAt"          orm:"deleted_at"          description:""`
	ChannelId          int64       `json:"channelId"          orm:"channel_id"          description:""`
	ChannelBlacklisted int         `json:"channelBlacklisted" orm:"channel_blacklisted" description:""`
	ChannelSuffix      string      `json:"channelSuffix"      orm:"channel_suffix"      description:""`
}
