// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentChannel is the golang structure of table hg_content_channel for DAO operations like Where/Data.
type ContentChannel struct {
	g.Meta          `orm:"table:hg_content_channel, do:true"`
	Id              any         // ID
	SourceChannelId any         // FeiNiu频道ID
	TgChatId        any         // TG Chat ID
	Title           any         // 频道标题
	Username        any         // 频道用户名
	InviteLink      any         // 邀请链接
	SourceType      any         // 来源类型
	PublicStatus    any         // 前台公开状态
	AuthStatus      any         // 授权状态
	Remark          any         // 备注
	Status          any         // 状态
	CreatedAt       *gtime.Time // 创建时间
	UpdatedAt       *gtime.Time // 更新时间
	DeletedAt       *gtime.Time // 删除时间
}
