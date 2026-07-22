// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTenantVip is the golang structure for table youban_publish_tenant_vip.
type YoubanPublishTenantVip struct {
	Id        int64       `json:"id"        orm:"id"         description:""`
	TenantId  int64       `json:"tenantId"  orm:"tenant_id"  description:""`
	Level     int         `json:"level"     orm:"level"      description:""`
	Status    int         `json:"status"    orm:"status"     description:""`
	OpenedAt  *gtime.Time `json:"openedAt"  orm:"opened_at"  description:""`
	ExpiredAt *gtime.Time `json:"expiredAt" orm:"expired_at" description:""`
	Remark    string      `json:"remark"    orm:"remark"     description:""`
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" description:""`
	UpdatedAt *gtime.Time `json:"updatedAt" orm:"updated_at" description:""`
	DeletedAt *gtime.Time `json:"deletedAt" orm:"deleted_at" description:""`
}
