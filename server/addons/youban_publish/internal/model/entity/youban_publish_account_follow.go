// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishAccountFollow is the golang structure for table youban_publish_account_follow.
type YoubanPublishAccountFollow struct {
	Id                       uint64      `json:"id"                       orm:"id"                         description:"主键"`
	TenantId                 int64       `json:"tenantId"                 orm:"tenant_id"                  description:"租户ID"`
	FollowerAccountId        int64       `json:"followerAccountId"        orm:"follower_account_id"        description:"关注人账号ID"`
	FollowingAccountId       int64       `json:"followingAccountId"       orm:"following_account_id"       description:"被关注账号ID"`
	Status                   string      `json:"status"                   orm:"status"                     description:"状态"`
	ApprovalRequiredSnapshot int         `json:"approvalRequiredSnapshot" orm:"approval_required_snapshot" description:"申请时是否需要审批"`
	Remark                   string      `json:"remark"                   orm:"remark"                     description:"备注"`
	BlockedBy                int64       `json:"blockedBy"                orm:"blocked_by"                 description:"拉黑人"`
	ApprovedAt               *gtime.Time `json:"approvedAt"               orm:"approved_at"                description:"通过时间"`
	CreatedAt                *gtime.Time `json:"createdAt"                orm:"created_at"                 description:"创建时间"`
	UpdatedAt                *gtime.Time `json:"updatedAt"                orm:"updated_at"                 description:"更新时间"`
	DeletedAt                *gtime.Time `json:"deletedAt"                orm:"deleted_at"                 description:"删除时间"`
}
