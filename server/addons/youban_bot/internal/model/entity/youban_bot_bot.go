// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotBot is the golang structure for table youban_bot_bot.
type YoubanBotBot struct {
	Id          int64       `json:"id"          orm:"id"           ` //
	BotName     string      `json:"botName"     orm:"bot_name"     ` //
	BotUsername string      `json:"botUsername" orm:"bot_username" ` //
	BotToken    string      `json:"botToken"    orm:"bot_token"    ` //
	IsOfficial  int         `json:"isOfficial"  orm:"is_official"  ` //
	IsDefault   int         `json:"isDefault"   orm:"is_default"   ` //
	RunMode     string      `json:"runMode"     orm:"run_mode"     ` //
	WebhookUrl  string      `json:"webhookUrl"  orm:"webhook_url"  ` //
	Remark      string      `json:"remark"      orm:"remark"       ` //
	Status      int         `json:"status"      orm:"status"       ` //
	CreatedBy   int64       `json:"createdBy"   orm:"created_by"   ` //
	UpdatedBy   int64       `json:"updatedBy"   orm:"updated_by"   ` //
	DeletedBy   int64       `json:"deletedBy"   orm:"deleted_by"   ` //
	CreatedAt   *gtime.Time `json:"createdAt"   orm:"created_at"   ` //
	UpdatedAt   *gtime.Time `json:"updatedAt"   orm:"updated_at"   ` //
	DeletedAt   *gtime.Time `json:"deletedAt"   orm:"deleted_at"   ` //
}
