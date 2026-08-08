package member

import (
	"context"

	v1 "hotgo/api/api/member/v1"
	"hotgo/internal/service"
)

func (c *ControllerV1) Register(ctx context.Context, req *v1.RegisterReq) (res *v1.RegisterRes, err error) {
	model, err := service.AdminSite().MobileRegister(ctx, &req.MemberRegisterInp)
	if err != nil {
		return
	}
	res = &v1.RegisterRes{LoginModel: model}
	return
}

func (c *ControllerV1) AccountLogin(ctx context.Context, req *v1.AccountLoginReq) (res *v1.AccountLoginRes, err error) {
	model, err := service.AdminSite().MemberAccountLogin(ctx, &req.AccountLoginInp)
	if err != nil {
		return
	}
	res = &v1.AccountLoginRes{LoginModel: model}
	return
}

func (c *ControllerV1) Info(ctx context.Context, req *v1.InfoReq) (res *v1.InfoRes, err error) {
	data, err := service.AdminMember().LoginMemberInfo(ctx)
	if err != nil {
		return
	}
	res = new(v1.InfoRes)
	res.LoginMemberInfoModel = data
	return
}
