// Package member
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package v1

import (
	"hotgo/internal/model/input/adminin"
	"hotgo/internal/model/input/form"
	"hotgo/internal/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

// GetIdByCodeReq 通过邀请码获取用户ID
type GetIdByCodeReq struct {
	g.Meta `path:"/member/getIdByCode" method:"post" tags:"用户" summary:"通过邀请码获取用户ID"`
	Code   string `json:"code"   dc:"邀请码"`
}

type GetIdByCodeRes struct{}

type RegisterReq struct {
	g.Meta `path:"/member/register" method:"post" tags:"移动端用户" summary:"账号注册"`
	adminin.MemberRegisterInp
}

type RegisterRes struct {
	*adminin.LoginModel
}

type AccountLoginReq struct {
	g.Meta `path:"/member/accountLogin" method:"post" tags:"移动端用户" summary:"账号密码登录"`
	adminin.AccountLoginInp
}

type AccountLoginRes struct {
	*adminin.LoginModel
}

type InfoReq struct {
	g.Meta `path:"/member/info" method:"get" tags:"移动端用户" summary:"获取登录用户信息"`
}

type InfoRes struct {
	*adminin.LoginMemberInfoModel
}

type MemberVipPayReq struct {
	g.Meta `path:"/member/vip/pay" method:"post" tags:"移动端用户" summary:"创建会员认证支付订单"`
	adminin.MemberVipPayCreateInp
}

type MemberVipPayRes struct {
	*adminin.MemberVipPayCreateModel
}

type MemberVipConfigReq struct {
	g.Meta `path:"/member/vip/config" method:"get" tags:"移动端用户" summary:"获取会员认证支付配置"`
}

type MemberVipConfigRes struct {
	*adminin.MemberVipConfigModel
}

type UpdateProfileReq struct {
	g.Meta `path:"/member/profile/update" method:"post" tags:"移动端用户" summary:"更新登录用户资料"`
	adminin.MemberUpdateProfileInp
}

type UpdateProfileRes struct{}

type UpdatePasswordReq struct {
	g.Meta      `path:"/member/password/update" method:"post" tags:"移动端用户" summary:"修改登录用户密码"`
	OldPassword string `json:"oldPassword" v:"required#原密码不能为空" dc:"原密码"`
	NewPassword string `json:"newPassword" v:"required#新密码不能为空" dc:"新密码"`
}

type UpdatePasswordRes struct{}

type SettingsReq struct {
	g.Meta `path:"/member/settings" method:"get" tags:"移动端用户" summary:"获取个人设置"`
	sysin.MemberSettingsInp
}

type SettingsRes struct {
	*sysin.MemberSettingsModel
}

type UpdateSettingsReq struct {
	g.Meta `path:"/member/settings/update" method:"post" tags:"移动端用户" summary:"更新个人设置"`
	sysin.MemberSettingsEditInp
}

type UpdateSettingsRes struct{}

type FavoriteListReq struct {
	g.Meta `path:"/member/favorites" method:"get" tags:"移动端用户" summary:"获取我的收藏资料"`
	sysin.MemberFavoriteListInp
}

type FavoriteListRes struct {
	form.PageRes
	List []*sysin.ContentProfileListModel `json:"list" dc:"收藏资料列表"`
}

type BlockedProfileListReq struct {
	g.Meta `path:"/member/blocked/profiles" method:"get" tags:"移动端用户" summary:"获取我的拉黑资料"`
	sysin.MemberBlockedProfileListInp
}

type BlockedProfileListRes struct {
	form.PageRes
	List []*sysin.ContentProfileListModel `json:"list" dc:"拉黑资料列表"`
}

type FavoriteToggleReq struct {
	g.Meta `path:"/member/favorite/toggle" method:"post" tags:"移动端用户" summary:"收藏或取消收藏资料"`
	sysin.MemberFavoriteToggleInp
}

type FavoriteToggleRes struct {
	*sysin.MemberFavoriteToggleModel
}

type FavoriteIdsReq struct {
	g.Meta `path:"/member/favorite/ids" method:"get" tags:"移动端用户" summary:"获取我的收藏资料ID"`
	sysin.MemberFavoriteIdsInp
}

type FavoriteIdsRes struct {
	*sysin.MemberFavoriteIdsModel
}

type ProfileRelationReq struct {
	g.Meta `path:"/member/profile/relation" method:"get" tags:"移动端用户" summary:"获取资料关系"`
	sysin.MemberProfileRelationInp
}

type ProfileRelationRes struct {
	*sysin.MemberProfileRelationModel
}

type BlockProfileReq struct {
	g.Meta `path:"/member/profile/block" method:"post" tags:"移动端用户" summary:"拉黑资料"`
	sysin.MemberProfileActionInp
}

type BlockProfileRes struct{}

type UnblockProfileReq struct {
	g.Meta `path:"/member/profile/unblock" method:"post" tags:"移动端用户" summary:"解除拉黑资料"`
	sysin.MemberProfileActionInp
}

type UnblockProfileRes struct{}

type RejectProfileReq struct {
	g.Meta `path:"/member/profile/reject" method:"post" tags:"移动端用户" summary:"拒绝资料"`
	sysin.MemberProfileActionInp
}

type RejectProfileRes struct{}

type ImmersiveProfileListReq struct {
	g.Meta `path:"/member/immersive/profiles" method:"get" tags:"移动端用户" summary:"获取沉浸模式资料列表"`
	sysin.MemberImmersiveProfileListInp
}

type ImmersiveProfileListRes struct {
	form.PageRes
	List []*sysin.ContentProfileListModel `json:"list" dc:"沉浸模式资料列表"`
}

type TraceListReq struct {
	g.Meta `path:"/member/traces" method:"get" tags:"移动端用户" summary:"获取我的浏览痕迹"`
	sysin.MemberProfileTraceListInp
}

type TraceListRes struct {
	form.PageRes
	List []*sysin.ContentProfileListModel `json:"list" dc:"浏览痕迹资料列表"`
}

type TraceRecordReq struct {
	g.Meta `path:"/member/trace/record" method:"post" tags:"移动端用户" summary:"记录资料浏览痕迹"`
	sysin.MemberProfileTraceRecordInp
}

type TraceRecordRes struct{}

type StatsReq struct {
	g.Meta `path:"/member/stats" method:"get" tags:"移动端用户" summary:"获取我的统计"`
	sysin.MemberStatsInp
}

type StatsRes struct {
	*sysin.MemberStatsModel
}

type AgreementReq struct {
	g.Meta `path:"/member/agreement" method:"get" tags:"移动端用户" summary:"获取移动端协议"`
	sysin.MemberAgreementInp
}

type AgreementRes struct {
	*sysin.MemberAgreementModel
}

type ShareCreateReq struct {
	g.Meta `path:"/member/share/create" method:"post" tags:"移动端用户" summary:"创建资料分享链接"`
	sysin.MemberShareCreateInp
}

type ShareCreateRes struct {
	*sysin.MemberShareCreateModel
}

type ShareOpenReq struct {
	g.Meta `path:"/member/share/open" method:"post" tags:"移动端用户" summary:"打开资料分享链接"`
	sysin.MemberShareOpenInp
}

type ShareOpenRes struct {
	*sysin.MemberShareOpenModel
}
