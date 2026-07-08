// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgQueueStat is the golang structure for table youban_publish_tg_queue_stat.
type YoubanPublishTgQueueStat struct {
	Id            int64       `json:"id"            orm:"id"             description:""`
	StatTime      *gtime.Time `json:"statTime"      orm:"stat_time"      description:""`
	QueueName     string      `json:"queueName"     orm:"queue_name"     description:""`
	PriorityLevel int         `json:"priorityLevel" orm:"priority_level" description:""`
	Status        string      `json:"status"        orm:"status"         description:""`
	JobCount      int         `json:"jobCount"      orm:"job_count"      description:""`
	OldestJobAt   *gtime.Time `json:"oldestJobAt"   orm:"oldest_job_at"  description:""`
	LatestJobAt   *gtime.Time `json:"latestJobAt"   orm:"latest_job_at"  description:""`
	CreatedAt     *gtime.Time `json:"createdAt"     orm:"created_at"     description:""`
	UpdatedAt     *gtime.Time `json:"updatedAt"     orm:"updated_at"     description:""`
}
