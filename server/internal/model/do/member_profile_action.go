package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type MemberProfileAction struct {
	g.Meta     `orm:"table:hg_member_profile_action, do:true"`
	Id         any
	MemberId   any
	ProfileId  any
	ActionType any
	CreatedAt  *gtime.Time
	UpdatedAt  *gtime.Time
	DeletedAt  *gtime.Time
}
