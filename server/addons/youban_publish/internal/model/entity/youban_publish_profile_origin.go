// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishProfileOrigin is the golang structure for table youban_publish_profile_origin.
type YoubanPublishProfileOrigin struct {
	Id              int64       `json:"id"              orm:"id"                ` //
	TenantId        int64       `json:"tenantId"        orm:"tenant_id"         ` //
	AccountId       int64       `json:"accountId"       orm:"account_id"        ` //
	ProfileId       int64       `json:"profileId"       orm:"profile_id"        ` //
	OriginKey       string      `json:"originKey"       orm:"origin_key"        ` //
	OriginKind      string      `json:"originKind"      orm:"origin_kind"       ` //
	CollectMethod   string      `json:"collectMethod"   orm:"collect_method"    ` //
	SourceId        int64       `json:"sourceId"        orm:"source_id"         ` //
	EventId         int64       `json:"eventId"         orm:"event_id"          ` //
	BotId           int64       `json:"botId"           orm:"bot_id"            ` //
	TgAccountId     int64       `json:"tgAccountId"     orm:"tg_account_id"     ` //
	FollowAccountId int64       `json:"followAccountId" orm:"follow_account_id" ` //
	ChatIdRaw       string      `json:"chatIdRaw"       orm:"chat_id_raw"       ` //
	ChatId          string      `json:"chatId"          orm:"chat_id"           ` //
	MessageId       int64       `json:"messageId"       orm:"message_id"        ` //
	GroupedId       string      `json:"groupedId"       orm:"grouped_id"        ` //
	ChannelTitle    string      `json:"channelTitle"    orm:"channel_title"     ` //
	ChannelUsername string      `json:"channelUsername" orm:"channel_username"  ` //
	ReceivedAt      *gtime.Time `json:"receivedAt"      orm:"received_at"       ` //
	EvidenceKind    string      `json:"evidenceKind"    orm:"evidence_kind"     ` //
	Completeness    string      `json:"completeness"    orm:"completeness"      ` //
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        ` //
}
