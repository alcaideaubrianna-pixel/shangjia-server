// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishAccountFollow is the golang structure of table hg_youban_publish_account_follow for DAO operations like Where/Data.
type YoubanPublishAccountFollow struct {
	g.Meta                   `orm:"table:hg_youban_publish_account_follow, do:true"`
	Id                       any         // 主键
	TenantId                 any         // 租户ID
	FollowerAccountId        any         // 关注人账号ID
	FollowingAccountId       any         // 被关注账号ID
	Status                   any         // 状态
	ApprovalRequiredSnapshot any         // 申请时是否需要审批
	Remark                   any         // 备注
	BlockedBy                any         // 拉黑人
	ApprovedAt               *gtime.Time // 通过时间
	CreatedAt                *gtime.Time // 创建时间
	UpdatedAt                *gtime.Time // 更新时间
	DeletedAt                *gtime.Time // 删除时间
}
