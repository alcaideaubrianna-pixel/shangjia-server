package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type MemberSetting struct {
	g.Meta          `orm:"table:hg_member_setting, do:true"`
	Id              any
	MemberId        any
	MessageEnabled  any
	HideOnline      any
	HideViewHistory any
	MatchChatOnly   any
	ProfileScope    any
	PhotoScope      any
	ThemeMode       any
	CreatedAt       *gtime.Time
	UpdatedAt       *gtime.Time
	DeletedAt       *gtime.Time
}
