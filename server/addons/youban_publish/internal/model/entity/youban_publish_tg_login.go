// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgLogin is the golang structure for table youban_publish_tg_login.
type YoubanPublishTgLogin struct {
	Id               int64       `json:"id"               orm:"id"                description:""`
	MerchantId       int64       `json:"merchantId"       orm:"merchant_id"       description:""`
	AccountId        int64       `json:"accountId"        orm:"account_id"        description:""`
	LoginToken       string      `json:"loginToken"       orm:"login_token"       description:""`
	QrUrl            string      `json:"qrUrl"            orm:"qr_url"            description:""`
	TelegramUserId   string      `json:"telegramUserId"   orm:"telegram_user_id"  description:""`
	TelegramUsername string      `json:"telegramUsername" orm:"telegram_username" description:""`
	SessionKey       string      `json:"sessionKey"       orm:"session_key"       description:""`
	Status           string      `json:"status"           orm:"status"            description:""`
	ErrorMessage     string      `json:"errorMessage"     orm:"error_message"     description:""`
	ExpiresAt        *gtime.Time `json:"expiresAt"        orm:"expires_at"        description:""`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"        description:""`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"        description:""`
	TenantId         int64       `json:"tenantId"         orm:"tenant_id"         description:""`
}
