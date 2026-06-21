package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type MemberShare struct {
	g.Meta        `orm:"table:hg_member_share, do:true"`
	Id            any
	MemberId      any
	ProfileId     any
	ShareToken    any
	VisitCount    any
	RegisterCount any
	LastVisitAt   *gtime.Time
	CreatedAt     *gtime.Time
	UpdatedAt     *gtime.Time
	DeletedAt     *gtime.Time
}
