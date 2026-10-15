// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayDelivery is the golang structure for table youban_bot_relay_delivery.
type YoubanBotRelayDelivery struct {
	Id              int64       `json:"id"              orm:"id"                ` //
	BotId           int64       `json:"botId"           orm:"bot_id"            ` //
	BindingId       int64       `json:"bindingId"       orm:"binding_id"        ` //
	SourceChatId    string      `json:"sourceChatId"    orm:"source_chat_id"    ` //
	SourceMessageId int64       `json:"sourceMessageId" orm:"source_message_id" ` //
	MediaGroupId    string      `json:"mediaGroupId"    orm:"media_group_id"    ` //
	TargetChatId    string      `json:"targetChatId"    orm:"target_chat_id"    ` //
	Kind            string      `json:"kind"            orm:"kind"              ` //
	PairKey         string      `json:"pairKey"         orm:"pair_key"          ` //
	TopicTitle      string      `json:"topicTitle"      orm:"topic_title"       ` //
	RawJson         string      `json:"rawJson"         orm:"raw_json"          ` //
	Status          string      `json:"status"          orm:"status"            ` //
	Attempts        int         `json:"attempts"        orm:"attempts"          ` //
	AvailableAt     *gtime.Time `json:"availableAt"     orm:"available_at"      ` //
	LeaseToken      string      `json:"leaseToken"      orm:"lease_token"       ` //
	TargetMessageId int64       `json:"targetMessageId" orm:"target_message_id" ` //
	ErrorMessage    string      `json:"errorMessage"    orm:"error_message"     ` //
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        ` //
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        ` //
}
