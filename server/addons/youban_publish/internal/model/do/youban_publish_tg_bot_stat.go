// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgBotStat is the golang structure of table hg_youban_publish_tg_bot_stat for DAO operations like Where/Data.
type YoubanPublishTgBotStat struct {
	g.Meta           `orm:"table:hg_youban_publish_tg_bot_stat, do:true"`
	Id               any         //
	TenantId         any         //
	BotId            any         //
	BotName          any         //
	BotUsername      any         //
	PendingCount     any         //
	QueuedCount      any         //
	SendingCount     any         //
	SentCount        any         //
	FailedCount      any         //
	RetryCount       any         //
	RateLimitCount   any         //
	LastSentAt       *gtime.Time //
	LastErrorAt      *gtime.Time //
	LastErrorMessage any         //
	CreatedAt        *gtime.Time //
	UpdatedAt        *gtime.Time //
}
