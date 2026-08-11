// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorDelivery is the golang structure of table hg_tg_collector_delivery for DAO operations like Where/Data.
type TgCollectorDelivery struct {
	g.Meta       `orm:"table:hg_tg_collector_delivery, do:true"`
	Id           any         //
	TenantId     any         //
	EventId      any         //
	DeliveryKey  any         //
	Status       any         //
	Priority     any         //
	Payload      *gjson.Json //
	AttemptCount any         //
	NextRunAt    *gtime.Time //
	LeaseOwner   any         //
	LeaseUntil   *gtime.Time //
	ErrorMessage any         //
	CreatedAt    *gtime.Time //
	UpdatedAt    *gtime.Time //
}
