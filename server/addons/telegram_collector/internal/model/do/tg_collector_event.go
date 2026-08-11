// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorEvent is the golang structure of table hg_tg_collector_event for DAO operations like Where/Data.
type TgCollectorEvent struct {
	g.Meta       `orm:"table:hg_tg_collector_event, do:true"`
	Id           any         //
	TenantId     any         //
	SourceId     any         //
	SourceType   any         //
	BotKey       any         //
	AccountId    any         //
	ChatId       any         //
	MessageId    any         //
	UpdateId     any         //
	EventKey     any         //
	RawUpdate    *gjson.Json //
	Priority     any         //
	Status       any         //
	AttemptCount any         //
	NextRunAt    *gtime.Time //
	LeaseOwner   any         //
	LeaseUntil   *gtime.Time //
	ReceivedAt   *gtime.Time //
	ProcessedAt  *gtime.Time //
	ErrorMessage any         //
	CreatedAt    *gtime.Time //
	UpdatedAt    *gtime.Time //
}
