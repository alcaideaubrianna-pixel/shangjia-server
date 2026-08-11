// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorSource is the golang structure of table hg_tg_collector_source for DAO operations like Where/Data.
type TgCollectorSource struct {
	g.Meta         `orm:"table:hg_tg_collector_source, do:true"`
	Id             any         //
	TenantId       any         //
	AccountId      any         //
	BotId          any         //
	SourceType     any         //
	ChatId         any         //
	ChatTitle      any         //
	ChatUsername   any         //
	Status         any         //
	HistoryEnabled any         //
	HistoryCursor  any         //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
}
