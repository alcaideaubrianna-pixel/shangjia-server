package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type MemberFavorite struct {
	g.Meta    `orm:"table:hg_member_favorite, do:true"`
	Id        any
	MemberId  any
	ProfileId any
	CreatedAt *gtime.Time
	UpdatedAt *gtime.Time
	DeletedAt *gtime.Time
}
