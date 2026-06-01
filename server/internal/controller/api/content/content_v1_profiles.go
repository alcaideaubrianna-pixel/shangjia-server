package content

import (
	"context"
	v1 "hotgo/api/api/content/v1"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

func (c *ControllerV1) ListProfiles(ctx context.Context, req *v1.ListProfilesReq) (res *v1.ListProfilesRes, err error) {
	list, totalCount, err := service.SysContent().ListProfiles(ctx, &req.ContentProfileListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ContentProfileListModel{}
	}
	res = new(v1.ListProfilesRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *ControllerV1) ViewProfile(ctx context.Context, req *v1.ViewProfileReq) (res *v1.ViewProfileRes, err error) {
	data, err := service.SysContent().ViewProfile(ctx, &req.ContentProfileViewInp)
	if err != nil {
		return
	}
	res = new(v1.ViewProfileRes)
	res.ContentProfileViewModel = data
	return
}
