package sysin

import (
	"context"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/errors/gerror"
)

const (
	MemberThemeSystem = "system"
	MemberThemeLight  = "light"
	MemberThemeDark   = "dark"
)

var validMemberScopes = map[string]struct{}{
	"all":      {},
	"verified": {},
	"matched":  {},
}

var validMemberThemes = map[string]struct{}{
	MemberThemeSystem: {},
	MemberThemeLight:  {},
	MemberThemeDark:   {},
}

type MemberSettingsInp struct{}

type MemberSettingsModel struct {
	MessageEnabled  int    `json:"messageEnabled"  dc:"新消息提醒"`
	HideOnline      int    `json:"hideOnline"      dc:"隐藏在线状态"`
	HideViewHistory int    `json:"hideViewHistory" dc:"隐藏浏览记录"`
	MatchChatOnly   int    `json:"matchChatOnly"   dc:"仅匹配后聊天"`
	ProfileScope    string `json:"profileScope"    dc:"资料可见范围"`
	PhotoScope      string `json:"photoScope"      dc:"照片可见范围"`
	ThemeMode       string `json:"themeMode"       dc:"主题模式"`
}

type MemberSettingsEditInp struct {
	MessageEnabled  int    `json:"messageEnabled"  v:"in:0,1#新消息提醒参数错误"      dc:"新消息提醒"`
	HideOnline      int    `json:"hideOnline"      v:"in:0,1#隐藏在线参数错误"        dc:"隐藏在线状态"`
	HideViewHistory int    `json:"hideViewHistory" v:"in:0,1#浏览记录参数错误"        dc:"隐藏浏览记录"`
	MatchChatOnly   int    `json:"matchChatOnly"   v:"in:0,1#聊天权限参数错误"        dc:"仅匹配后聊天"`
	ProfileScope    string `json:"profileScope"                                    dc:"资料可见范围"`
	PhotoScope      string `json:"photoScope"                                      dc:"照片可见范围"`
	ThemeMode       string `json:"themeMode"                                       dc:"主题模式"`
}

func (in *MemberSettingsEditInp) Filter(ctx context.Context) (err error) {
	if _, ok := validMemberScopes[in.ProfileScope]; !ok {
		return gerror.New("资料可见范围不正确")
	}
	if _, ok := validMemberScopes[in.PhotoScope]; !ok {
		return gerror.New("照片可见范围不正确")
	}
	if _, ok := validMemberThemes[in.ThemeMode]; !ok {
		return gerror.New("主题模式不正确")
	}
	return
}

type MemberFavoriteListInp struct {
	form.PageReq
}

type MemberBlockedProfileListInp struct {
	form.PageReq
}

type MemberFavoriteToggleInp struct {
	ProfileId int64 `json:"profileId" v:"required|min:1#资料ID不能为空|资料ID不能为空" dc:"资料ID"`
}

type MemberFavoriteToggleModel struct {
	Favorited bool `json:"favorited" dc:"是否已收藏"`
}

type MemberFavoriteIdsInp struct{}

type MemberFavoriteIdsModel struct {
	Ids []int64 `json:"ids" dc:"收藏资料ID"`
}

type MemberProfileRelationInp struct {
	ProfileId int64 `json:"profileId" v:"required|min:1#资料ID不能为空|资料ID不能为空" dc:"资料ID"`
}

type MemberProfileRelationModel struct {
	Favorited bool `json:"favorited" dc:"是否已喜欢"`
	Blocked   bool `json:"blocked"   dc:"是否已拉黑"`
	Rejected  bool `json:"rejected"  dc:"是否已拒绝"`
}

type MemberProfileActionInp struct {
	ProfileId int64 `json:"profileId" v:"required|min:1#资料ID不能为空|资料ID不能为空" dc:"资料ID"`
}

type MemberImmersiveProfileListInp struct {
	ContentProfileListInp
}

type MemberStatsInp struct{}

type MemberStatsModel struct {
	FavoriteCount int `json:"favoriteCount" dc:"收藏数"`
	ContactCount  int `json:"contactCount"  dc:"联系数"`
	TraceCount    int `json:"traceCount"    dc:"痕迹数"`
	MatchCount    int `json:"matchCount"    dc:"兼容字段，等同痕迹数"`
}

type MemberAgreementInp struct {
	Type string `json:"type" v:"required#协议类型不能为空" dc:"协议类型"`
}

type MemberAgreementModel struct {
	Type    string `json:"type"    dc:"协议类型"`
	Title   string `json:"title"   dc:"标题"`
	Content string `json:"content" dc:"内容"`
}

type MemberShareCreateInp struct {
	ProfileId int64 `json:"profileId" v:"required|min:1#资料ID不能为空|资料ID不能为空" dc:"资料ID"`
}

type MemberShareCreateModel struct {
	ShareToken string `json:"shareToken" dc:"分享TOKEN"`
	ShareUrl   string `json:"shareUrl"   dc:"分享地址"`
}

type MemberShareOpenInp struct {
	ShareToken string `json:"shareToken" v:"required#分享TOKEN不能为空" dc:"分享TOKEN"`
}

type MemberShareOpenModel struct {
	ProfileId  int64  `json:"profileId"  dc:"资料ID"`
	InviteCode string `json:"inviteCode" dc:"邀请人邀请码"`
}

type MemberShareRegisterInp struct {
	ShareToken string
	MemberId   int64
}

type MemberProfileTraceListInp struct {
	form.PageReq
}

type MemberProfileTraceRecordInp struct {
	ProfileId int64 `json:"profileId" v:"required|min:1#资料ID不能为空|资料ID不能为空" dc:"资料ID"`
}
