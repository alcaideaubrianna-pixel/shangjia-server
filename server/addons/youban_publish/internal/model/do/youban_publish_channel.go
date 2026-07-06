// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishChannel is the golang structure of table hg_youban_publish_channel for DAO operations like Where/Data.
type YoubanPublishChannel struct {
	g.Meta              `orm:"table:hg_youban_publish_channel, do:true"`
	Id                  any         // 主键
	TenantId            any         // 租户ID
	MerchantId          any         // 兼容旧版本商家ID
	TgAccountId         any         // TG账号ID
	ChannelTitle        any         // 频道名称
	ChannelUsername     any         // 频道用户名
	TargetChatId        any         // 目标Chat ID
	PublishDirection    any         // 上架/下架频道
	CyclePublishEnabled any         // 是否循环上架
	CyclePublishDays    any         // 循环上架天数
	CyclePublishTime    any         // 循环上架时间
	IsDefaultSelected   any         // 是否默认选中
	BotIdJson           any         // 绑定Bot ID JSON
	Remark              any         // 备注
	Status              any         // 状态
	LastRefreshStatus   any         // 最近刷新状态
	LastRefreshMessage  any         // 最近刷新信息
	LastRefreshAt       *gtime.Time // 最近刷新时间
	CreatedBy           any         // 创建人
	UpdatedBy           any         // 更新人
	DeletedBy           any         // 删除人
	CreatedAt           *gtime.Time // 创建时间
	UpdatedAt           *gtime.Time // 更新时间
	DeletedAt           *gtime.Time // 删除时间
}
