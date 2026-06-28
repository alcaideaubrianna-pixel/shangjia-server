// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgLogin is the golang structure for table youban_publish_tg_login.
type YoubanPublishTgLogin struct {
	Id               uint64      `json:"id"               orm:"id"                description:"主键"`
	MerchantId       int64       `json:"merchantId"       orm:"merchant_id"       description:"商家ID"`
	AccountId        int64       `json:"accountId"        orm:"account_id"        description:"账号ID"`
	LoginToken       string      `json:"loginToken"       orm:"login_token"       description:"登录令牌"`
	QrUrl            string      `json:"qrUrl"            orm:"qr_url"            description:"二维码地址"`
	TelegramUserId   string      `json:"telegramUserId"   orm:"telegram_user_id"  description:"TG用户ID"`
	TelegramUsername string      `json:"telegramUsername" orm:"telegram_username" description:"TG用户名"`
	SessionKey       string      `json:"sessionKey"       orm:"session_key"       description:"会话存储键"`
	Status           string      `json:"status"           orm:"status"            description:"状态"`
	ErrorMessage     string      `json:"errorMessage"     orm:"error_message"     description:"错误信息"`
	ExpiresAt        *gtime.Time `json:"expiresAt"        orm:"expires_at"        description:"过期时间"`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"        description:"创建时间"`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"        description:"更新时间"`
}
