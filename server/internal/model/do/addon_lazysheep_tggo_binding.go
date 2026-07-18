// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoBinding is the golang structure of table hg_addon_lazysheep_tggo_binding for DAO operations like Where/Data.
type AddonLazysheepTggoBinding struct {
	g.Meta          `orm:"table:hg_addon_lazysheep_tggo_binding, do:true"`
	Id              any         // 主键
	BindingKey      any         // 绑定标识
	BotId           any         // 机器人ID
	BotKey          any         // 机器人标识
	SourceUrl       any         // BangChat 链接
	SourceToken     any         // BangChat token
	SourceRoomId    any         // 来源房间ID
	SourcePairId    any         // 来源 pairId
	ReviewChatId    any         // 审核群ID
	PublishChatId   any         // 推送频道ID
	AutoPush        any         // 自动推送
	ReviewEnabled   any         // 审核开关
	PublishEnabled  any         // 推送开关
	VerifyEnabled   any         // 验证按钮开关
	LocationEnabled any         // 位置按钮开关
	LastPullId      any         // 最后拉取ID
	LastCursor      any         // 最后游标
	Status          any         // 状态
	CreatedBy       any         // 创建者
	UpdatedBy       any         // 更新者
	CreatedAt       *gtime.Time // 创建时间
	UpdatedAt       *gtime.Time // 更新时间
}
