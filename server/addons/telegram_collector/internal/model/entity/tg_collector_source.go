// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorSource is the golang structure for table tg_collector_source.
type TgCollectorSource struct {
	Id             int64       `json:"id"             orm:"id"              description:""`
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"       description:""`
	AccountId      int64       `json:"accountId"      orm:"account_id"      description:""`
	BotId          int64       `json:"botId"          orm:"bot_id"          description:""`
	SourceType     string      `json:"sourceType"     orm:"source_type"     description:""`
	ChatId         string      `json:"chatId"         orm:"chat_id"         description:""`
	ChatTitle      string      `json:"chatTitle"      orm:"chat_title"      description:""`
	ChatUsername   string      `json:"chatUsername"   orm:"chat_username"   description:""`
	Status         string      `json:"status"         orm:"status"          description:""`
	HistoryEnabled int         `json:"historyEnabled" orm:"history_enabled" description:""`
	HistoryCursor  string      `json:"historyCursor"  orm:"history_cursor"  description:""`
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"      description:""`
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"      description:""`
}
