// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorAccountTask is the golang structure of table hg_tg_collector_account_task for DAO operations like Where/Data.
type TgCollectorAccountTask struct {
	g.Meta       `orm:"table:hg_tg_collector_account_task, do:true"`
	Id           any         //
	TenantId     any         //
	AccountId    any         //
	TaskType     any         //
	TaskKey      any         //
	Priority     any         //
	Status       any         //
	Payload      *gjson.Json //
	Result       *gjson.Json //
	AttemptCount any         //
	MaxAttempts  any         //
	NextRunAt    *gtime.Time //
	LeaseOwner   any         //
	LeaseEpoch   any         //
	LeaseUntil   *gtime.Time //
	ErrorMessage any         //
	CompletedAt  *gtime.Time //
	CreatedAt    *gtime.Time //
	UpdatedAt    *gtime.Time //
}
