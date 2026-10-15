// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayConfig is the golang structure for table youban_bot_relay_config.
type YoubanBotRelayConfig struct {
	Id            int64       `json:"id"            orm:"id"             ` //
	BotId         int64       `json:"botId"         orm:"bot_id"         ` //
	OwnerId       int64       `json:"ownerId"       orm:"owner_id"       ` //
	Enabled       int         `json:"enabled"       orm:"enabled"        ` //
	BlockLinks    int         `json:"blockLinks"    orm:"block_links"    ` //
	AuditDatabase int         `json:"auditDatabase" orm:"audit_database" ` //
	AuditGroupId  string      `json:"auditGroupId"  orm:"audit_group_id" ` //
	CreatedAt     *gtime.Time `json:"createdAt"     orm:"created_at"     ` //
	UpdatedAt     *gtime.Time `json:"updatedAt"     orm:"updated_at"     ` //
}
