// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgJobLog is the golang structure for table youban_publish_tg_job_log.
type YoubanPublishTgJobLog struct {
	Id        uint64      `json:"id"        orm:"id"         description:"主键"`
	JobId     int64       `json:"jobId"     orm:"job_id"     description:"TG任务ID"`
	TaskId    int64       `json:"taskId"    orm:"task_id"    description:"任务ID"`
	TenantId  int64       `json:"tenantId"  orm:"tenant_id"  description:"租户ID"`
	AccountId int64       `json:"accountId" orm:"account_id" description:"账号ID"`
	ProfileId int64       `json:"profileId" orm:"profile_id" description:"资料ID"`
	BotId     int64       `json:"botId"     orm:"bot_id"     description:"Bot ID"`
	Action    string      `json:"action"    orm:"action"     description:"动作"`
	Status    string      `json:"status"    orm:"status"     description:"状态"`
	Message   string      `json:"message"   orm:"message"    description:"日志内容"`
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" description:"创建时间"`
}
