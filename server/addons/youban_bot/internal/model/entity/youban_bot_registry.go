// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRegistry is the golang structure for table youban_bot_registry.
type YoubanBotRegistry struct {
	Id                   int64       `json:"id"                   orm:"id"                     ` //
	TenantId             int64       `json:"tenantId"             orm:"tenant_id"              ` //
	AccountId            int64       `json:"accountId"            orm:"account_id"             ` //
	BotType              string      `json:"botType"              orm:"bot_type"               ` //
	BotName              string      `json:"botName"              orm:"bot_name"               ` //
	BotUsername          string      `json:"botUsername"          orm:"bot_username"           ` //
	BotToken             string      `json:"botToken"             orm:"bot_token"              ` //
	BotUserId            string      `json:"botUserId"            orm:"bot_user_id"            ` //
	IsOfficial           int         `json:"isOfficial"           orm:"is_official"            ` //
	IsDefault            int         `json:"isDefault"            orm:"is_default"             ` //
	RunMode              string      `json:"runMode"              orm:"run_mode"               ` //
	WebhookUrl           string      `json:"webhookUrl"           orm:"webhook_url"            ` //
	Remark               string      `json:"remark"               orm:"remark"                 ` //
	Status               int         `json:"status"               orm:"status"                 ` //
	CreatedBy            int64       `json:"createdBy"            orm:"created_by"             ` //
	UpdatedBy            int64       `json:"updatedBy"            orm:"updated_by"             ` //
	DeletedBy            int64       `json:"deletedBy"            orm:"deleted_by"             ` //
	TgAccountId          int64       `json:"tgAccountId"          orm:"tg_account_id"          ` //
	WelcomeMessage       string      `json:"welcomeMessage"       orm:"welcome_message"        ` //
	SupergroupId         string      `json:"supergroupId"         orm:"supergroup_id"          ` //
	SupergroupAccessHash string      `json:"supergroupAccessHash" orm:"supergroup_access_hash" ` //
	SupergroupTitle      string      `json:"supergroupTitle"      orm:"supergroup_title"       ` //
	InviteLink           string      `json:"inviteLink"           orm:"invite_link"            ` //
	SetupStatus          string      `json:"setupStatus"          orm:"setup_status"           ` //
	WebhookStatus        string      `json:"webhookStatus"        orm:"webhook_status"         ` //
	ErrorMessage         string      `json:"errorMessage"         orm:"error_message"          ` //
	LastSetupAt          *gtime.Time `json:"lastSetupAt"          orm:"last_setup_at"          ` //
	LastWebhookAt        *gtime.Time `json:"lastWebhookAt"        orm:"last_webhook_at"        ` //
	CreatedAt            *gtime.Time `json:"createdAt"            orm:"created_at"             ` //
	UpdatedAt            *gtime.Time `json:"updatedAt"            orm:"updated_at"             ` //
	DeletedAt            *gtime.Time `json:"deletedAt"            orm:"deleted_at"             ` //
}
