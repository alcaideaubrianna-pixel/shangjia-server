// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgMessage is the golang structure for table youban_publish_tg_message.
type YoubanPublishTgMessage struct {
	Id           uint64      `json:"id"           orm:"id"             description:"主键"`
	JobId        int64       `json:"jobId"        orm:"job_id"         description:"TG任务ID"`
	TaskId       int64       `json:"taskId"       orm:"task_id"        description:"任务ID"`
	TenantId     int64       `json:"tenantId"     orm:"tenant_id"      description:"租户ID"`
	AccountId    int64       `json:"accountId"    orm:"account_id"     description:"账号ID"`
	ProfileId    int64       `json:"profileId"    orm:"profile_id"     description:"资料ID"`
	BotId        int64       `json:"botId"        orm:"bot_id"         description:"Bot ID"`
	TargetChatId string      `json:"targetChatId" orm:"target_chat_id" description:"目标Chat ID"`
	TgMessageId  int64       `json:"tgMessageId"  orm:"tg_message_id"  description:"TG消息ID"`
	MediaGroupId string      `json:"mediaGroupId" orm:"media_group_id" description:"媒体组ID"`
	MediaId      int64       `json:"mediaId"      orm:"media_id"       description:"媒体ID"`
	Purpose      string      `json:"purpose"      orm:"purpose"        description:"display/verify"`
	TgFileId     string      `json:"tgFileId"     orm:"tg_file_id"     description:"TG文件ID"`
	Status       string      `json:"status"       orm:"status"         description:"状态"`
	SentAt       *gtime.Time `json:"sentAt"       orm:"sent_at"        description:"发送时间"`
	DeletedAt    *gtime.Time `json:"deletedAt"    orm:"deleted_at"     description:"删除时间"`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"     description:"创建时间"`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"     description:"更新时间"`
}
