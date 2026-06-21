package member

import (
	"context"
	v1 "hotgo/api/api/member/v1"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model/input/adminin"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
	"hotgo/utility/simple"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) UpdateProfile(ctx context.Context, req *v1.UpdateProfileReq) (res *v1.UpdateProfileRes, err error) {
	err = service.AdminMember().UpdateProfile(ctx, &req.MemberUpdateProfileInp)
	if err != nil {
		return
	}
	res = new(v1.UpdateProfileRes)
	return
}

func (c *ControllerV1) UpdatePassword(ctx context.Context, req *v1.UpdatePasswordReq) (res *v1.UpdatePasswordRes, err error) {
	memberId := contexts.GetUserId(ctx)
	if memberId <= 0 {
		err = gerror.New("请先登录")
		return
	}
	oldPassword, err := simple.DecryptText(req.OldPassword)
	if err != nil {
		err = gerror.New("原密码格式不正确")
		return
	}
	newPassword, err := simple.DecryptText(req.NewPassword)
	if err != nil {
		err = gerror.New("新密码格式不正确")
		return
	}
	if err = g.Validator().Data(newPassword).Rules("password").Messages("密码长度在6~18之间").Run(ctx); err != nil {
		return
	}
	err = service.AdminMember().UpdatePwd(ctx, &adminin.MemberUpdatePwdInp{
		Id:          memberId,
		OldPassword: oldPassword,
		NewPassword: newPassword,
	})
	if err != nil {
		return
	}
	res = new(v1.UpdatePasswordRes)
	return
}

func (c *ControllerV1) Settings(ctx context.Context, req *v1.SettingsReq) (res *v1.SettingsRes, err error) {
	data, err := service.SysMemberApp().Settings(ctx, &req.MemberSettingsInp)
	if err != nil {
		return
	}
	res = &v1.SettingsRes{MemberSettingsModel: data}
	return
}

func (c *ControllerV1) UpdateSettings(ctx context.Context, req *v1.UpdateSettingsReq) (res *v1.UpdateSettingsRes, err error) {
	err = service.SysMemberApp().UpdateSettings(ctx, &req.MemberSettingsEditInp)
	if err != nil {
		return
	}
	res = new(v1.UpdateSettingsRes)
	return
}

func (c *ControllerV1) FavoriteList(ctx context.Context, req *v1.FavoriteListReq) (res *v1.FavoriteListRes, err error) {
	list, totalCount, err := service.SysMemberApp().FavoriteList(ctx, &req.MemberFavoriteListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ContentProfileListModel{}
	}
	res = new(v1.FavoriteListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *ControllerV1) BlockedProfileList(ctx context.Context, req *v1.BlockedProfileListReq) (res *v1.BlockedProfileListRes, err error) {
	list, totalCount, err := service.SysMemberApp().BlockedProfileList(ctx, &req.MemberBlockedProfileListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ContentProfileListModel{}
	}
	res = new(v1.BlockedProfileListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *ControllerV1) FavoriteToggle(ctx context.Context, req *v1.FavoriteToggleReq) (res *v1.FavoriteToggleRes, err error) {
	data, err := service.SysMemberApp().FavoriteToggle(ctx, &req.MemberFavoriteToggleInp)
	if err != nil {
		return
	}
	res = &v1.FavoriteToggleRes{MemberFavoriteToggleModel: data}
	return
}

func (c *ControllerV1) FavoriteIds(ctx context.Context, req *v1.FavoriteIdsReq) (res *v1.FavoriteIdsRes, err error) {
	data, err := service.SysMemberApp().FavoriteIds(ctx, &req.MemberFavoriteIdsInp)
	if err != nil {
		return
	}
	res = &v1.FavoriteIdsRes{MemberFavoriteIdsModel: data}
	return
}

func (c *ControllerV1) ProfileRelation(ctx context.Context, req *v1.ProfileRelationReq) (res *v1.ProfileRelationRes, err error) {
	data, err := service.SysMemberApp().ProfileRelation(ctx, &req.MemberProfileRelationInp)
	if err != nil {
		return
	}
	res = &v1.ProfileRelationRes{MemberProfileRelationModel: data}
	return
}

func (c *ControllerV1) BlockProfile(ctx context.Context, req *v1.BlockProfileReq) (res *v1.BlockProfileRes, err error) {
	err = service.SysMemberApp().BlockProfile(ctx, &req.MemberProfileActionInp)
	if err != nil {
		return
	}
	res = new(v1.BlockProfileRes)
	return
}

func (c *ControllerV1) UnblockProfile(ctx context.Context, req *v1.UnblockProfileReq) (res *v1.UnblockProfileRes, err error) {
	err = service.SysMemberApp().UnblockProfile(ctx, &req.MemberProfileActionInp)
	if err != nil {
		return
	}
	res = new(v1.UnblockProfileRes)
	return
}

func (c *ControllerV1) RejectProfile(ctx context.Context, req *v1.RejectProfileReq) (res *v1.RejectProfileRes, err error) {
	err = service.SysMemberApp().RejectProfile(ctx, &req.MemberProfileActionInp)
	if err != nil {
		return
	}
	res = new(v1.RejectProfileRes)
	return
}

func (c *ControllerV1) ImmersiveProfileList(ctx context.Context, req *v1.ImmersiveProfileListReq) (res *v1.ImmersiveProfileListRes, err error) {
	list, totalCount, err := service.SysMemberApp().ImmersiveProfileList(ctx, &req.MemberImmersiveProfileListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ContentProfileListModel{}
	}
	res = new(v1.ImmersiveProfileListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *ControllerV1) TraceList(ctx context.Context, req *v1.TraceListReq) (res *v1.TraceListRes, err error) {
	list, totalCount, err := service.SysMemberApp().TraceList(ctx, &req.MemberProfileTraceListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ContentProfileListModel{}
	}
	res = new(v1.TraceListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *ControllerV1) TraceRecord(ctx context.Context, req *v1.TraceRecordReq) (res *v1.TraceRecordRes, err error) {
	err = service.SysMemberApp().TraceRecord(ctx, &req.MemberProfileTraceRecordInp)
	if err != nil {
		return
	}
	res = new(v1.TraceRecordRes)
	return
}

func (c *ControllerV1) Stats(ctx context.Context, req *v1.StatsReq) (res *v1.StatsRes, err error) {
	data, err := service.SysMemberApp().Stats(ctx, &req.MemberStatsInp)
	if err != nil {
		return
	}
	res = &v1.StatsRes{MemberStatsModel: data}
	return
}

func (c *ControllerV1) Agreement(ctx context.Context, req *v1.AgreementReq) (res *v1.AgreementRes, err error) {
	data, err := service.SysMemberApp().Agreement(ctx, &req.MemberAgreementInp)
	if err != nil {
		return
	}
	res = &v1.AgreementRes{MemberAgreementModel: data}
	return
}

func (c *ControllerV1) ShareCreate(ctx context.Context, req *v1.ShareCreateReq) (res *v1.ShareCreateRes, err error) {
	data, err := service.SysMemberApp().CreateShare(ctx, &req.MemberShareCreateInp)
	if err != nil {
		return
	}
	res = &v1.ShareCreateRes{MemberShareCreateModel: data}
	return
}

func (c *ControllerV1) ShareOpen(ctx context.Context, req *v1.ShareOpenReq) (res *v1.ShareOpenRes, err error) {
	data, err := service.SysMemberApp().OpenShare(ctx, &req.MemberShareOpenInp)
	if err != nil {
		return
	}
	res = &v1.ShareOpenRes{MemberShareOpenModel: data}
	return
}
