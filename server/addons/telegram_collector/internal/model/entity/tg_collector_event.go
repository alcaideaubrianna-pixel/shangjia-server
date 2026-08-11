// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorEvent is the golang structure for table tg_collector_event.
type TgCollectorEvent struct {
	Id           int64       `json:"id"           orm:"id"            description:""`
	TenantId     int64       `json:"tenantId"     orm:"tenant_id"     description:""`
	SourceId     int64       `json:"sourceId"     orm:"source_id"     description:""`
	SourceType   string      `json:"sourceType"   orm:"source_type"   description:""`
	BotKey       string      `json:"botKey"       orm:"bot_key"       description:""`
	AccountId    int64       `json:"accountId"    orm:"account_id"    description:""`
	ChatId       string      `json:"chatId"       orm:"chat_id"       description:""`
	MessageId    int64       `json:"messageId"    orm:"message_id"    description:""`
	UpdateId     int64       `json:"updateId"     orm:"update_id"     description:""`
	EventKey     string      `json:"eventKey"     orm:"event_key"     description:""`
	RawUpdate    *gjson.Json `json:"rawUpdate"    orm:"raw_update"    description:""`
	Priority     int         `json:"priority"     orm:"priority"      description:""`
	Status       string      `json:"status"       orm:"status"        description:""`
	AttemptCount int         `json:"attemptCount" orm:"attempt_count" description:""`
	NextRunAt    *gtime.Time `json:"nextRunAt"    orm:"next_run_at"   description:""`
	LeaseOwner   string      `json:"leaseOwner"   orm:"lease_owner"   description:""`
	LeaseUntil   *gtime.Time `json:"leaseUntil"   orm:"lease_until"   description:""`
	ReceivedAt   *gtime.Time `json:"receivedAt"   orm:"received_at"   description:""`
	ProcessedAt  *gtime.Time `json:"processedAt"  orm:"processed_at"  description:""`
	ErrorMessage string      `json:"errorMessage" orm:"error_message" description:""`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"    description:""`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"    description:""`
}
