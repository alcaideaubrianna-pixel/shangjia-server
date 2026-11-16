// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgAttempt is the golang structure of table hg_youban_publish_tg_attempt for DAO operations like Where/Data.
type YoubanPublishTgAttempt struct {
	g.Meta           `orm:"table:hg_youban_publish_tg_attempt, do:true"`
	Id               any         //
	JobId            any         //
	TenantId         any         //
	BotId            any         //
	ChannelId        any         //
	TargetChatId     any         //
	Phase            any         //
	ChunkIndex       any         //
	AttemptNo        any         //
	AttemptToken     any         //
	ExpectedCount    any         //
	ConfirmedCount   any         //
	Status           any         //
	ResponseReceived any         //
	WebhookDeadline  *gtime.Time //
	ErrorMessage     any         //
	CreatedAt        *gtime.Time //
	UpdatedAt        *gtime.Time //
}
