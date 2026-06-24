package member

import (
	"context"

	v1 "hotgo/api/api/member/v1"
	"hotgo/internal/service"
)

func (c *ControllerV1) MemberVipConfig(ctx context.Context, req *v1.MemberVipConfigReq) (res *v1.MemberVipConfigRes, err error) {
	data, err := service.AdminOrder().GetMemberVipPayConfig(ctx)
	if err != nil {
		return
	}
	res = &v1.MemberVipConfigRes{MemberVipConfigModel: data}
	return
}
