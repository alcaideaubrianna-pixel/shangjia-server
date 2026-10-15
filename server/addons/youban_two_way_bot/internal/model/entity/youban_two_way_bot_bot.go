// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotBot is the golang structure for table youban_two_way_bot_bot.
type YoubanTwoWayBotBot struct {
	Id                   int64       `json:"id"                   orm:"id"                     description:""`
	TenantId             int64       `json:"tenantId"             orm:"tenant_id"              description:""`
	AccountId            int64       `json:"accountId"            orm:"account_id"             description:""`
	TgAccountId          int64       `json:"tgAccountId"          orm:"tg_account_id"          description:""`
	Name                 string      `json:"name"                 orm:"name"                   description:""`
	BotToken             string      `json:"botToken"             orm:"bot_token"              description:""`
	BotUserId            string      `json:"botUserId"            orm:"bot_user_id"            description:""`
	BotUsername          string      `json:"botUsername"          orm:"bot_username"           description:""`
	SupergroupId         string      `json:"supergroupId"         orm:"supergroup_id"          description:""`
	SupergroupAccessHash string      `json:"supergroupAccessHash" orm:"supergroup_access_hash" description:""`
	SupergroupTitle      string      `json:"supergroupTitle"      orm:"supergroup_title"       description:""`
	InviteLink           string      `json:"inviteLink"           orm:"invite_link"            description:""`
	SetupStatus          string      `json:"setupStatus"          orm:"setup_status"           description:""`
	WebhookStatus        string      `json:"webhookStatus"        orm:"webhook_status"         description:""`
	Status               int         `json:"status"               orm:"status"                 description:""`
	ErrorMessage         string      `json:"errorMessage"         orm:"error_message"          description:""`
	LastSetupAt          *gtime.Time `json:"lastSetupAt"          orm:"last_setup_at"          description:""`
	LastWebhookAt        *gtime.Time `json:"lastWebhookAt"        orm:"last_webhook_at"        description:""`
	CreatedAt            *gtime.Time `json:"createdAt"            orm:"created_at"             description:""`
	UpdatedAt            *gtime.Time `json:"updatedAt"            orm:"updated_at"             description:""`
	DeletedAt            *gtime.Time `json:"deletedAt"            orm:"deleted_at"             description:""`
	WelcomeMessage       string      `json:"welcomeMessage"       orm:"welcome_message"        description:""`
}
