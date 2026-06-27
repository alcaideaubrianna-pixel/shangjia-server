package content

import (
	"context"

	v1 "hotgo/api/api/content/v1"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

func (c *ControllerV1) ListAnnouncements(ctx context.Context, req *v1.ListAnnouncementsReq) (res *v1.ListAnnouncementsRes, err error) {
	list, totalCount, err := service.SysAppAnnouncement().PublicList(ctx, &req.AppAnnouncementPublicListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AppAnnouncementPublicListModel{}
	}
	res = new(v1.ListAnnouncementsRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *ControllerV1) ViewAnnouncement(ctx context.Context, req *v1.ViewAnnouncementReq) (res *v1.ViewAnnouncementRes, err error) {
	data, err := service.SysAppAnnouncement().PublicView(ctx, &req.AppAnnouncementPublicViewInp)
	if err != nil {
		return
	}
	res = new(v1.ViewAnnouncementRes)
	res.AppAnnouncementPublicListModel = data
	return
}

func (c *ControllerV1) ListArticleCategories(ctx context.Context, req *v1.ListArticleCategoriesReq) (res *v1.ListArticleCategoriesRes, err error) {
	list, err := service.SysAppAnnouncement().PublicCategories(ctx)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.AppAnnouncementCategoryModel{}
	}
	res = new(v1.ListArticleCategoriesRes)
	res.List = list
	return
}

func (c *ControllerV1) SeoFooter(ctx context.Context, req *v1.SeoFooterReq) (res *v1.SeoFooterRes, err error) {
	data, err := service.SysAppAnnouncement().SeoFooter(ctx)
	if err != nil {
		return
	}
	res = new(v1.SeoFooterRes)
	res.SeoFooterModel = data
	return
}
