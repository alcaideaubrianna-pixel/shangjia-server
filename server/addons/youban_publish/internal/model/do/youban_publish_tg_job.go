// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgJob is the golang structure of table hg_youban_publish_tg_job for DAO operations like Where/Data.
type YoubanPublishTgJob struct {
	g.Meta       `orm:"table:hg_youban_publish_tg_job, do:true"`
	Id           any         // 主键
	TaskId       any         // 任务ID
	MerchantId   any         // 商家ID
	AccountId    any         // 账号ID
	ProfileId    any         // 资料ID
	BotId        any         // Bot ID
	TargetChatId any         // 目标Chat ID
	TgMessageId  any         // TG消息ID
	Status       any         // 状态
	RetryCount   any         // 重试次数
	NextRetryAt  *gtime.Time // 下次重试时间
	ErrorMessage any         // 错误信息
	CreatedAt    *gtime.Time // 创建时间
	UpdatedAt    *gtime.Time // 更新时间
}
