package model

import "github.com/gogf/gf/v2/os/gtime"

// BotRuntime is the two-way Bot projection stored in hg_youban_publish_bot.
type BotRuntime struct {
	Id                   int64       `json:"id"                    orm:"id"`
	TenantId             int64       `json:"tenantId"              orm:"tenant_id"`
	AccountId            int64       `json:"accountId"             orm:"account_id"`
	TgAccountId          int64       `json:"tgAccountId"           orm:"tg_account_id"`
	Name                 string      `json:"name"                  orm:"name"`
	BotToken             string      `json:"botToken"              orm:"bot_token"`
	BotUserId            string      `json:"botUserId"             orm:"bot_user_id"`
	BotUsername          string      `json:"botUsername"           orm:"bot_username"`
	SupergroupId         string      `json:"supergroupId"          orm:"supergroup_id"`
	SupergroupAccessHash string      `json:"supergroupAccessHash"  orm:"supergroup_access_hash"`
	SupergroupTitle      string      `json:"supergroupTitle"       orm:"supergroup_title"`
	InviteLink           string      `json:"inviteLink"            orm:"invite_link"`
	SetupStatus          string      `json:"setupStatus"           orm:"setup_status"`
	WebhookStatus        string      `json:"webhookStatus"         orm:"webhook_status"`
	Status               int         `json:"status"                orm:"status"`
	ErrorMessage         string      `json:"errorMessage"          orm:"error_message"`
	LastSetupAt          *gtime.Time `json:"lastSetupAt"           orm:"last_setup_at"`
	LastWebhookAt        *gtime.Time `json:"lastWebhookAt"         orm:"last_webhook_at"`
	CreatedAt            *gtime.Time `json:"createdAt"             orm:"created_at"`
	UpdatedAt            *gtime.Time `json:"updatedAt"             orm:"updated_at"`
	DeletedAt            *gtime.Time `json:"deletedAt"             orm:"deleted_at"`
	WelcomeMessage       string      `json:"welcomeMessage"        orm:"welcome_message"`
}
