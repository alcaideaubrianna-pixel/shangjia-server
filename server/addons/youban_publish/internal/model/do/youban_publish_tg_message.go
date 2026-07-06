// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgMessage is the golang structure of table hg_youban_publish_tg_message for DAO operations like Where/Data.
type YoubanPublishTgMessage struct {
	g.Meta       `orm:"table:hg_youban_publish_tg_message, do:true"`
	Id           any         // 主键
	JobId        any         // TG任务ID
	TaskId       any         // 任务ID
	TenantId     any         // 租户ID
	AccountId    any         // 账号ID
	ProfileId    any         // 资料ID
	BotId        any         // Bot ID
	TargetChatId any         // 目标Chat ID
	TgMessageId  any         // TG消息ID
	MediaGroupId any         // 媒体组ID
	MediaId      any         // 媒体ID
	Purpose      any         // display/verify
	TgFileId     any         // TG文件ID
	Status       any         // 状态
	SentAt       *gtime.Time // 发送时间
	DeletedAt    *gtime.Time // 删除时间
	CreatedAt    *gtime.Time // 创建时间
	UpdatedAt    *gtime.Time // 更新时间
}
