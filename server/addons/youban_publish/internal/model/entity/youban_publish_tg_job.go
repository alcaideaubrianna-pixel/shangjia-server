// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgJob is the golang structure for table youban_publish_tg_job.
type YoubanPublishTgJob struct {
	Id           uint64      `json:"id"           orm:"id"             description:"主键"`
	TaskId       int64       `json:"taskId"       orm:"task_id"        description:"任务ID"`
	MerchantId   int64       `json:"merchantId"   orm:"merchant_id"    description:"商家ID"`
	AccountId    int64       `json:"accountId"    orm:"account_id"     description:"账号ID"`
	ProfileId    int64       `json:"profileId"    orm:"profile_id"     description:"资料ID"`
	BotId        int64       `json:"botId"        orm:"bot_id"         description:"Bot ID"`
	TargetChatId string      `json:"targetChatId" orm:"target_chat_id" description:"目标Chat ID"`
	TgMessageId  int64       `json:"tgMessageId"  orm:"tg_message_id"  description:"TG消息ID"`
	Status       string      `json:"status"       orm:"status"         description:"状态"`
	RetryCount   int         `json:"retryCount"   orm:"retry_count"    description:"重试次数"`
	NextRetryAt  *gtime.Time `json:"nextRetryAt"  orm:"next_retry_at"  description:"下次重试时间"`
	ErrorMessage string      `json:"errorMessage" orm:"error_message"  description:"错误信息"`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"     description:"创建时间"`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"     description:"更新时间"`
}
