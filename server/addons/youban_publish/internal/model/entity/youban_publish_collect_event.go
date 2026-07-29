// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectEvent is the golang structure for table youban_publish_collect_event.
type YoubanPublishCollectEvent struct {
	Id                    uint64      `json:"id"              orm:"id"                description:"主键"`
	TenantId              int64       `json:"tenantId"        orm:"tenant_id"         description:"租户ID"`
	AccountId             int64       `json:"accountId"       orm:"account_id"        description:"所属账号ID"`
	SourceId              int64       `json:"sourceId"        orm:"source_id"         description:"采集源ID"`
	SourceType            string      `json:"sourceType"      orm:"source_type"       description:"来源类型"`
	BotId                 int64       `json:"botId"           orm:"bot_id"            description:"机器人ID"`
	TgAccountId           int64       `json:"tgAccountId"     orm:"tg_account_id"     description:"协议号ID"`
	SourceChatId          string      `json:"sourceChatId"    orm:"source_chat_id"    description:"来源频道/群聊ID"`
	SourceMessageId       int64       `json:"sourceMessageId" orm:"source_message_id" description:"来源消息ID"`
	SourceGroupedId       string      `json:"sourceGroupedId" orm:"source_grouped_id" description:"媒体组ID"`
	SourceUniqueKey       string      `json:"sourceUniqueKey" orm:"source_unique_key" description:"来源唯一键"`
	MaterialRole          string      `json:"materialRole" orm:"material_role" description:"资料组角色"`
	MaterialParentEventId int64       `json:"materialParentEventId" orm:"material_parent_event_id" description:"验证资料所属展示事件ID"`
	MaterialGroupStatus   string      `json:"materialGroupStatus" orm:"material_group_status" description:"资料组状态"`
	RawText               string      `json:"rawText"         orm:"raw_text"          description:"原始文本"`
	MediaCount            int         `json:"mediaCount"      orm:"media_count"       description:"媒体数量"`
	MediaJson             string      `json:"mediaJson"       orm:"media_json"        description:"媒体JSON"`
	TextHash              string      `json:"textHash"        orm:"text_hash"         description:"文本哈希"`
	DedupeKey             string      `json:"dedupeKey"       orm:"dedupe_key"        description:"去重键"`
	Status                string      `json:"status"          orm:"status"            description:"状态"`
	ErrorMessage          string      `json:"errorMessage"    orm:"error_message"     description:"错误信息"`
	ReceivedAt            *gtime.Time `json:"receivedAt"      orm:"received_at"       description:"接收时间"`
	ProcessedAt           *gtime.Time `json:"processedAt"     orm:"processed_at"      description:"处理时间"`
	CreatedAt             *gtime.Time `json:"createdAt"       orm:"created_at"        description:"创建时间"`
	UpdatedAt             *gtime.Time `json:"updatedAt"       orm:"updated_at"        description:"更新时间"`
}
