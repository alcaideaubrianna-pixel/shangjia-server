// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotCooperationApplication is the golang structure of table hg_youban_two_way_bot_cooperation_application for DAO operations like Where/Data.
type YoubanTwoWayBotCooperationApplication struct {
	g.Meta               `orm:"table:hg_youban_two_way_bot_cooperation_application, do:true"`
	Id                   any         //
	TenantId             any         //
	ConfigId             any         //
	ApplicantTgUserId    any         //
	ApplicantUsername    any         //
	ApplicantFirstName   any         //
	ApplicantLastName    any         //
	SubmittedBotUserId   any         //
	SubmittedBotUsername any         //
	SubmittedBotName     any         //
	ReviewStatus         any         //
	JoinStatus           any         //
	TopicThreadId        any         //
	ReviewedBy           any         //
	ReviewRemark         any         //
	ErrorMessage         any         //
	SubmittedAt          *gtime.Time //
	ReviewedAt           *gtime.Time //
	CreatedAt            *gtime.Time //
	UpdatedAt            *gtime.Time //
	DeletedAt            *gtime.Time //
}
