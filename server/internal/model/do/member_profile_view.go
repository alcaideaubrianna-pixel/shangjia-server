package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type MemberProfileView struct {
	g.Meta     `orm:"table:hg_member_profile_view, do:true"`
	Id         any
	MemberId   any
	ProfileId  any
	ViewCount  any
	LastViewAt *gtime.Time
	CreatedAt  *gtime.Time
	UpdatedAt  *gtime.Time
	DeletedAt  *gtime.Time
}
