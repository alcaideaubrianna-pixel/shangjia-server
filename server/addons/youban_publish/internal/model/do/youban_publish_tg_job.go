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
	g.Meta                 `orm:"table:hg_youban_publish_tg_job, do:true"`
	Id                     any         //
	TaskId                 any         //
	OperationNo            any         //
	TenantId               any         //
	MerchantId             any         //
	AccountId              any         //
	ProfileId              any         //
	ChannelId              any         //
	BotId                  any         //
	TargetChatId           any         //
	CollectEventId         any         //
	CollectSourceId        any         //
	CollectSourceChatId    any         //
	CollectSourceMessageId any         //
	TgMessageId            any         //
	AsynqTaskId            any         //
	Status                 any         //
	RetryCount             any         //
	NextRetryAt            *gtime.Time //
	SentAt                 *gtime.Time //
	CycleEnabled           any         //
	CycleDays              any         //
	CyclePublishTime       any         //
	NextCycleAt            *gtime.Time //
	Priority               any         //
	QueueName              any         //
	DispatchStatus         any         //
	DispatchedAt           *gtime.Time //
	DispatchCount          any         //
	SendPhase              any         //
	ReconcileCount         any         //
	LastDispatchError      any         //
	ErrorMessage           any         //
	CreatedAt              *gtime.Time //
	UpdatedAt              *gtime.Time //
	PushMode               any         //
	OperationType          any         //
	ProfileNoSnapshot      any         //
	TitleSnapshot          any         //
	NotifyAccountId        any         //
	CycleFailureCount      any         //
	SendDeadlineAt         *gtime.Time //
	TimeoutParentJobId     any         //
	TimeoutCleanupAt       *gtime.Time //
	TimeoutRetryCount      any         //
	DisplayMediaSelection  any         //
}
