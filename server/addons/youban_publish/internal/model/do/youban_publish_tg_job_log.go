// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgJobLog is the golang structure of table hg_youban_publish_tg_job_log for DAO operations like Where/Data.
type YoubanPublishTgJobLog struct {
	g.Meta    `orm:"table:hg_youban_publish_tg_job_log, do:true"`
	Id        any         // 主键
	JobId     any         // TG任务ID
	TaskId    any         // 任务ID
	TenantId  any         // 租户ID
	AccountId any         // 账号ID
	ProfileId any         // 资料ID
	BotId     any         // Bot ID
	Action    any         // 动作
	Status    any         // 状态
	Message   any         // 日志内容
	CreatedAt *gtime.Time // 创建时间
}
