// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgJob is the golang structure of table hg_youban_publish_tg_job for DAO operations like Where/Data.
type YoubanPublishTgJob struct {
	g.Meta           `orm:"table:hg_youban_publish_tg_job, do:true"`
	Id               any         //
	TaskId           any         //
	TenantId         any         //
	MerchantId       any         //
	AccountId        any         //
	ProfileId        any         //
	BotId            any         //
	TargetChatId     any         //
	TgMessageId      any         //
	Status           any         //
	RetryCount       any         //
	NextRetryAt      *gtime.Time //
	ErrorMessage     any         //
	CreatedAt        *gtime.Time //
	UpdatedAt        *gtime.Time //
	ChannelId        any         //
	SentAt           *gtime.Time //
	CycleEnabled     any         //
	CycleDays        any         //
	NextCycleAt      *gtime.Time //
	CyclePublishTime any         //
	AsynqTaskId      any         //
	OperationNo      any         //
}
