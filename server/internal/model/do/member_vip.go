package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type MemberVip struct {
	g.Meta    `orm:"table:hg_member_vip, do:true"`
	Id        any
	MemberId  any
	Level     any
	Status    any
	OpenedAt  *gtime.Time
	ExpiredAt *gtime.Time
	Remark    any
	CreatedAt *gtime.Time
	UpdatedAt *gtime.Time
	DeletedAt *gtime.Time
}
