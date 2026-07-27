// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotCooperationApplication is the golang structure for table youban_two_way_bot_cooperation_application.
type YoubanTwoWayBotCooperationApplication struct {
	Id                   int64       `json:"id"                   orm:"id"                     description:""`
	TenantId             int64       `json:"tenantId"             orm:"tenant_id"              description:""`
	ConfigId             int64       `json:"configId"             orm:"config_id"              description:""`
	ApplicantTgUserId    string      `json:"applicantTgUserId"    orm:"applicant_tg_user_id"   description:""`
	ApplicantUsername    string      `json:"applicantUsername"    orm:"applicant_username"     description:""`
	ApplicantFirstName   string      `json:"applicantFirstName"   orm:"applicant_first_name"   description:""`
	ApplicantLastName    string      `json:"applicantLastName"    orm:"applicant_last_name"    description:""`
	SubmittedBotUserId   string      `json:"submittedBotUserId"   orm:"submitted_bot_user_id"  description:""`
	SubmittedBotUsername string      `json:"submittedBotUsername" orm:"submitted_bot_username" description:""`
	SubmittedBotName     string      `json:"submittedBotName"     orm:"submitted_bot_name"     description:""`
	ReviewStatus         string      `json:"reviewStatus"         orm:"review_status"          description:""`
	JoinStatus           string      `json:"joinStatus"           orm:"join_status"            description:""`
	TopicThreadId        int64       `json:"topicThreadId"        orm:"topic_thread_id"        description:""`
	ReviewedBy           int64       `json:"reviewedBy"           orm:"reviewed_by"            description:""`
	ReviewRemark         string      `json:"reviewRemark"         orm:"review_remark"          description:""`
	ErrorMessage         string      `json:"errorMessage"         orm:"error_message"          description:""`
	SubmittedAt          *gtime.Time `json:"submittedAt"          orm:"submitted_at"           description:""`
	ReviewedAt           *gtime.Time `json:"reviewedAt"           orm:"reviewed_at"            description:""`
	CreatedAt            *gtime.Time `json:"createdAt"            orm:"created_at"             description:""`
	UpdatedAt            *gtime.Time `json:"updatedAt"            orm:"updated_at"             description:""`
	DeletedAt            *gtime.Time `json:"deletedAt"            orm:"deleted_at"             description:""`
}
