// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotCooperationBlacklist is the golang structure for table youban_two_way_bot_cooperation_blacklist.
type YoubanTwoWayBotCooperationBlacklist struct {
	Id                 int64       `json:"id"                 orm:"id"                   description:""`
	TenantId           int64       `json:"tenantId"           orm:"tenant_id"            description:""`
	ConfigId           int64       `json:"configId"           orm:"config_id"            description:""`
	ApplicantTgUserId  string      `json:"applicantTgUserId"  orm:"applicant_tg_user_id" description:""`
	ApplicantUsername  string      `json:"applicantUsername"  orm:"applicant_username"   description:""`
	ApplicantFirstName string      `json:"applicantFirstName" orm:"applicant_first_name" description:""`
	ApplicantLastName  string      `json:"applicantLastName"  orm:"applicant_last_name"  description:""`
	Reason             string      `json:"reason"             orm:"reason"               description:""`
	Status             int         `json:"status"             orm:"status"               description:""`
	CreatedBy          int64       `json:"createdBy"          orm:"created_by"           description:""`
	UpdatedBy          int64       `json:"updatedBy"          orm:"updated_by"           description:""`
	CreatedAt          *gtime.Time `json:"createdAt"          orm:"created_at"           description:""`
	UpdatedAt          *gtime.Time `json:"updatedAt"          orm:"updated_at"           description:""`
}
