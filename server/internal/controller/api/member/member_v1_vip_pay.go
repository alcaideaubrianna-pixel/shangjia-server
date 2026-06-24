package member

import (
	"context"

	v1 "hotgo/api/api/member/v1"
	"hotgo/internal/service"
)

func (c *ControllerV1) MemberVipPay(ctx context.Context, req *v1.MemberVipPayReq) (res *v1.MemberVipPayRes, err error) {
	data, err := service.AdminOrder().CreateMemberVipPay(ctx, &req.MemberVipPayCreateInp)
	if err != nil {
		return
	}
	res = &v1.MemberVipPayRes{MemberVipPayCreateModel: data}
	return
}
