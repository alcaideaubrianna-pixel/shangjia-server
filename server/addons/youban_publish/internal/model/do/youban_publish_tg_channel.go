// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgChannel is the golang structure of table hg_youban_publish_tg_channel for DAO operations like Where/Data.
type YoubanPublishTgChannel struct {
	g.Meta          `orm:"table:hg_youban_publish_tg_channel, do:true"`
	Id              any         // 主键
	TenantId        any         // 租户ID
	MerchantId      any         // 兼容旧版本商家ID
	TgAccountId     any         // TG账号ID
	ChannelId       any         // 频道ID
	AccessHash      any         // AccessHash
	ChannelTitle    any         // 频道名称
	ChannelUsername any         // 频道用户名
	IsBroadcast     any         // 是否频道
	IsMegagroup     any         // 是否群组
	CanPostMessages any         // 账号可发频道消息
	CanInviteUsers  any         // 账号可邀请用户
	CanAddAdmins    any         // 账号可添加管理员
	LastSyncAt      *gtime.Time // 最后同步时间
	CreatedAt       *gtime.Time // 创建时间
	UpdatedAt       *gtime.Time // 更新时间
}
