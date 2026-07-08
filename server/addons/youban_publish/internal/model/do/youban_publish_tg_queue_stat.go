// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgQueueStat is the golang structure of table hg_youban_publish_tg_queue_stat for DAO operations like Where/Data.
type YoubanPublishTgQueueStat struct {
	g.Meta        `orm:"table:hg_youban_publish_tg_queue_stat, do:true"`
	Id            any         //
	StatTime      *gtime.Time //
	QueueName     any         //
	PriorityLevel any         //
	Status        any         //
	JobCount      any         //
	OldestJobAt   *gtime.Time //
	LatestJobAt   *gtime.Time //
	CreatedAt     *gtime.Time //
	UpdatedAt     *gtime.Time //
}
