// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentChannel is the golang structure for table content_channel.
type ContentChannel struct {
	Id              int64       `json:"id"              orm:"id"                description:"ID"`
	SourceChannelId int64       `json:"sourceChannelId" orm:"source_channel_id" description:"FeiNiu频道ID"`
	TgChatId        string      `json:"tgChatId"        orm:"tg_chat_id"        description:"TG Chat ID"`
	Title           string      `json:"title"           orm:"title"             description:"频道标题"`
	Username        string      `json:"username"        orm:"username"          description:"频道用户名"`
	InviteLink      string      `json:"inviteLink"      orm:"invite_link"       description:"邀请链接"`
	SourceType      string      `json:"sourceType"      orm:"source_type"       description:"来源类型"`
	PublicStatus    string      `json:"publicStatus"    orm:"public_status"     description:"前台公开状态"`
	AuthStatus      string      `json:"authStatus"      orm:"auth_status"       description:"授权状态"`
	Remark          string      `json:"remark"          orm:"remark"            description:"备注"`
	Status          int         `json:"status"          orm:"status"            description:"状态"`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        description:"更新时间"`
	DeletedAt       *gtime.Time `json:"deletedAt"       orm:"deleted_at"        description:"删除时间"`
}
