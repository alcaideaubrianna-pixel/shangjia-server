// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorDelivery is the golang structure for table tg_collector_delivery.
type TgCollectorDelivery struct {
	Id           int64       `json:"id"           orm:"id"            description:""`
	TenantId     int64       `json:"tenantId"     orm:"tenant_id"     description:""`
	EventId      int64       `json:"eventId"      orm:"event_id"      description:""`
	DeliveryKey  string      `json:"deliveryKey"  orm:"delivery_key"  description:""`
	Status       string      `json:"status"       orm:"status"        description:""`
	Priority     int         `json:"priority"     orm:"priority"      description:""`
	Payload      *gjson.Json `json:"payload"      orm:"payload"       description:""`
	AttemptCount int         `json:"attemptCount" orm:"attempt_count" description:""`
	NextRunAt    *gtime.Time `json:"nextRunAt"    orm:"next_run_at"   description:""`
	LeaseOwner   string      `json:"leaseOwner"   orm:"lease_owner"   description:""`
	LeaseUntil   *gtime.Time `json:"leaseUntil"   orm:"lease_until"   description:""`
	ErrorMessage string      `json:"errorMessage" orm:"error_message" description:""`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"    description:""`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"    description:""`
}
