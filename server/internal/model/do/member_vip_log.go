package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type MemberVipLog struct {
	g.Meta          `orm:"table:hg_member_vip_log, do:true"`
	Id              any
	MemberId        any
	OperatorId      any
	Source          any
	Action          any
	BeforeStatus    any
	AfterStatus     any
	BeforeLevel     any
	AfterLevel      any
	BeforeExpiredAt *gtime.Time
	AfterExpiredAt  *gtime.Time
	Remark          any
	CreatedAt       *gtime.Time
}
