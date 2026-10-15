// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayAuditTopic is the golang structure for table youban_bot_relay_audit_topic.
type YoubanBotRelayAuditTopic struct {
	Id        int64       `json:"id"        orm:"id"         ` //
	BotId     int64       `json:"botId"     orm:"bot_id"     ` //
	GroupId   string      `json:"groupId"   orm:"group_id"   ` //
	PairKey   string      `json:"pairKey"   orm:"pair_key"   ` //
	ThreadId  int64       `json:"threadId"  orm:"thread_id"  ` //
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" ` //
	UpdatedAt *gtime.Time `json:"updatedAt" orm:"updated_at" ` //
}
