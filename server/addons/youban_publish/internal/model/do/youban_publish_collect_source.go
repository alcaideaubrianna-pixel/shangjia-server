// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectSource is the golang structure of table hg_youban_publish_collect_source for DAO operations like Where/Data.
type YoubanPublishCollectSource struct {
	g.Meta          `orm:"table:hg_youban_publish_collect_source, do:true"`
	Id              any         // 主键
	TenantId        any         // 租户ID
	AccountId       any         // 所属账号ID
	SourceType      any         // 来源类型
	Title           any         // 采集源名称
	SourceChatId    any         // 来源频道/群聊ID
	SourceUsername  any         // 来源用户名
	TgAccountId     any         // 协议号ID
	BotId           any         // 机器人ID
	FollowAccountId any         // 关注账号ID
	CollectEnabled  any         // 是否开启采集
	Status          any         // 状态
	EventTotal      any         // 事件总数
	SuccessTotal    any         // 成功数
	FailedTotal     any         // 失败数
	LastEventAt     *gtime.Time // 最后事件时间
	Remark          any         // 备注
	CreatedBy       any         // 创建人
	UpdatedBy       any         // 更新人
	DeletedBy       any         // 删除人
	CreatedAt       *gtime.Time // 创建时间
	UpdatedAt       *gtime.Time // 更新时间
	DeletedAt       *gtime.Time // 删除时间
}
