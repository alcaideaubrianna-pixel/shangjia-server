// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishBot is the golang structure for table youban_publish_bot.
type YoubanPublishBot struct {
	Id          uint64      `json:"id"          orm:"id"           description:"主键"`
	BotName     string      `json:"botName"     orm:"bot_name"     description:"Bot名称"`
	BotUsername string      `json:"botUsername" orm:"bot_username" description:"Bot用户名"`
	BotToken    string      `json:"botToken"    orm:"bot_token"    description:"Bot Token"`
	Remark      string      `json:"remark"      orm:"remark"       description:"备注"`
	Status      int         `json:"status"      orm:"status"       description:"状态"`
	CreatedBy   int64       `json:"createdBy"   orm:"created_by"   description:"创建人"`
	UpdatedBy   int64       `json:"updatedBy"   orm:"updated_by"   description:"更新人"`
	DeletedBy   int64       `json:"deletedBy"   orm:"deleted_by"   description:"删除人"`
	CreatedAt   *gtime.Time `json:"createdAt"   orm:"created_at"   description:"创建时间"`
	UpdatedAt   *gtime.Time `json:"updatedAt"   orm:"updated_at"   description:"更新时间"`
	DeletedAt   *gtime.Time `json:"deletedAt"   orm:"deleted_at"   description:"删除时间"`
}
