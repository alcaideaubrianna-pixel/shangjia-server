package entity

import "github.com/gogf/gf/v2/os/gtime"

type MemberSetting struct {
	Id              int64       `json:"id"              orm:"id"                description:"ID"`
	MemberId        int64       `json:"memberId"        orm:"member_id"         description:"用户ID"`
	MessageEnabled  int         `json:"messageEnabled"  orm:"message_enabled"   description:"新消息提醒"`
	HideOnline      int         `json:"hideOnline"      orm:"hide_online"       description:"隐藏在线状态"`
	HideViewHistory int         `json:"hideViewHistory" orm:"hide_view_history" description:"隐藏浏览记录"`
	MatchChatOnly   int         `json:"matchChatOnly"   orm:"match_chat_only"   description:"仅匹配后聊天"`
	ProfileScope    string      `json:"profileScope"    orm:"profile_scope"     description:"资料可见范围"`
	PhotoScope      string      `json:"photoScope"      orm:"photo_scope"       description:"照片可见范围"`
	ThemeMode       string      `json:"themeMode"       orm:"theme_mode"        description:"主题模式"`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        description:"更新时间"`
	DeletedAt       *gtime.Time `json:"deletedAt"       orm:"deleted_at"        description:"删除时间"`
}
