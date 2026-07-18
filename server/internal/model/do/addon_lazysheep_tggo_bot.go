// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoBot is the golang structure of table hg_addon_lazysheep_tggo_bot for DAO operations like Where/Data.
type AddonLazysheepTggoBot struct {
	g.Meta        `orm:"table:hg_addon_lazysheep_tggo_bot, do:true"`
	Id            any         // 主键
	BotKey        any         // 机器人标识
	MemberId      any         // 所属后台用户
	Token         any         // Telegram Bot Token
	BotName       any         // 机器人名称
	Username      any         // Telegram username
	WebhookSecret any         // Webhook Secret
	WebhookPath   any         // Webhook 路径
	Enabled       any         // 是否启用
	AutoPull      any         // 自动采集
	AutoForward   any         // 自动推送
	ReviewEnabled any         // 审核开关
	AllowVerify   any         // 允许查看验证
	AllowLocation any         // 允许查看位置
	MemberVerify  any         // 验证仅会员
	MemberPoints  any         // 积分解锁
	SignFollow    any         // 签到关注校验
	SignChannels  *gjson.Json // 签到必关频道
	ReviewText    any         // 审核文案模板
	PublishText   any         // 推送文案模板
	Sort          any         // 排序
	Status        any         // 状态
	CreatedBy     any         // 创建者
	UpdatedBy     any         // 更新者
	CreatedAt     *gtime.Time // 创建时间
	UpdatedAt     *gtime.Time // 更新时间
}
