// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTenantVipLog is the golang structure for table youban_publish_tenant_vip_log.
type YoubanPublishTenantVipLog struct {
	Id              int64       `json:"id"              orm:"id"                description:""`
	TenantId        int64       `json:"tenantId"        orm:"tenant_id"         description:""`
	OperatorId      int64       `json:"operatorId"      orm:"operator_id"       description:""`
	Source          string      `json:"source"          orm:"source"            description:""`
	Action          string      `json:"action"          orm:"action"            description:""`
	BeforeStatus    int         `json:"beforeStatus"    orm:"before_status"     description:""`
	BeforeLevel     int         `json:"beforeLevel"     orm:"before_level"      description:""`
	BeforeExpiredAt *gtime.Time `json:"beforeExpiredAt" orm:"before_expired_at" description:""`
	AfterStatus     int         `json:"afterStatus"     orm:"after_status"      description:""`
	AfterLevel      int         `json:"afterLevel"      orm:"after_level"       description:""`
	AfterExpiredAt  *gtime.Time `json:"afterExpiredAt"  orm:"after_expired_at"  description:""`
	Remark          string      `json:"remark"          orm:"remark"            description:""`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:""`
}
