package content

import (
	"context"
	v1 "hotgo/api/api/content/v1"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"

	"github.com/gogf/gf/v2/frame/g"
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

func (c *ControllerV1) HomeProfileCards(ctx context.Context, req *v1.HomeProfileCardsReq) (res *v1.HomeProfileCardsRes, err error) {
	list, totalCount, err := service.SysContent().HomeProfileCards(ctx, &req.HomeProfileCardsInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ContentProfileListModel{}
	}
	res = new(v1.HomeProfileCardsRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *ControllerV1) ImageSearch(ctx context.Context, req *v1.ImageSearchReq) (res *v1.ImageSearchRes, err error) {
	list, totalCount, err := service.SysContent().ImageSearch(ctx, &req.ContentProfileImageSearchInp, g.RequestFromCtx(ctx).GetUploadFile("image"))
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ContentProfileListModel{}
	}
	res = new(v1.ImageSearchRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *ControllerV1) FilterOptions(ctx context.Context, req *v1.FilterOptionsReq) (res *v1.FilterOptionsRes, err error) {
	data, err := service.SysContent().FilterOptions(ctx)
	if err != nil {
		return
	}
	res = new(v1.FilterOptionsRes)
	res.ContentProfileFilterOptionsModel = data
	return
}

func (c *ControllerV1) Regions(ctx context.Context, req *v1.RegionsReq) (res *v1.RegionsRes, err error) {
	data, err := service.SysContent().Regions(ctx)
	if err != nil {
		return
	}
	res = new(v1.RegionsRes)
	res.ContentProfileRegionsModel = data
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
