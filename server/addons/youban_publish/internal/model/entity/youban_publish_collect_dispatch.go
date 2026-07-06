// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectDispatch is the golang structure for table youban_publish_collect_dispatch.
type YoubanPublishCollectDispatch struct {
	Id                  uint64      `json:"id"                  orm:"id"                     description:"主键"`
	TenantId            int64       `json:"tenantId"            orm:"tenant_id"              description:"租户ID"`
	AccountId           int64       `json:"accountId"           orm:"account_id"             description:"所属账号ID"`
	SourceId            int64       `json:"sourceId"            orm:"source_id"              description:"采集源ID"`
	RuleId              int64       `json:"ruleId"              orm:"rule_id"                description:"规则ID"`
	EventId             int64       `json:"eventId"             orm:"event_id"               description:"采集事件ID"`
	ReviewId            int64       `json:"reviewId"            orm:"review_id"              description:"审核ID"`
	ProfileId           int64       `json:"profileId"           orm:"profile_id"             description:"资料ID"`
	TaskId              int64       `json:"taskId"              orm:"task_id"                description:"上架任务ID"`
	TargetChannelIdJson string      `json:"targetChannelIdJson" orm:"target_channel_id_json" description:"目标频道ID JSON"`
	BotIdJson           string      `json:"botIdJson"           orm:"bot_id_json"            description:"推送BOT ID JSON"`
	MatchJson           string      `json:"matchJson"           orm:"match_json"             description:"命中详情JSON"`
	Status              string      `json:"status"              orm:"status"                 description:"状态"`
	ErrorMessage        string      `json:"errorMessage"        orm:"error_message"          description:"错误信息"`
	CreatedAt           *gtime.Time `json:"createdAt"           orm:"created_at"             description:"创建时间"`
	UpdatedAt           *gtime.Time `json:"updatedAt"           orm:"updated_at"             description:"更新时间"`
	FinishedAt          *gtime.Time `json:"finishedAt"          orm:"finished_at"            description:"完成时间"`
}
