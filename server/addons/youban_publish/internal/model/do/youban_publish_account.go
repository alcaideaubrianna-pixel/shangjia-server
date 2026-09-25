// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishAccount is the golang structure of table hg_youban_publish_account for DAO operations like Where/Data.
type YoubanPublishAccount struct {
	g.Meta                 `orm:"table:hg_youban_publish_account, do:true"`
	Id                     any         // 主键
	TenantId               any         // 租户ID
	MerchantId             any         // 商家ID
	AdminMemberId          any         // 绑定系统账号ID
	ParentId               any         // 父账号ID
	AccountType            any         // 账号类型
	Nickname               any         // 昵称
	Username               any         // 用户名
	PasswordHash           any         // 密码hash
	Salt                   any         // 密码盐
	TelegramUserId         any         // TG用户ID
	TelegramUsername       any         // TG用户名
	DailyPublishLimit      any         // 每日上架额度
	CanDirectPublish       any         // 是否可直接发布
	AllowedChannelJson     any         // 可发布频道JSON
	AllowedRegionJson      any         // 可发布地区JSON
	Remark                 any         // 备注
	Status                 any         // 状态
	CreatedBy              any         // 创建人
	UpdatedBy              any         // 更新人
	DeletedBy              any         // 删除人
	CreatedAt              *gtime.Time // 创建时间
	UpdatedAt              *gtime.Time // 更新时间
	DeletedAt              *gtime.Time // 删除时间
	AvatarUrl              any         // 头像地址
	ContactTelegram        any         // 联系TG
	ContactWechat          any         // 联系微信
	ContactPhone           any         // 联系电话
	ContactOther           any         // 其他联系方式
	FollowApprovalRequired any         // 关注我是否需要审批
	PublicFollowEnabled    any         // 是否公开关注
}
