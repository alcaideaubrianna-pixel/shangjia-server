package sys

import (
	"context"
	"hotgo/api/admin/appannouncement"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

var AppAnnouncement = cAppAnnouncement{}

type cAppAnnouncement struct{}

func (c *cAppAnnouncement) List(ctx context.Context, req *appannouncement.ListReq) (res *appannouncement.ListRes, err error) {
	list, totalCount, err := service.SysAppAnnouncement().List(ctx, &req.AppAnnouncementListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AppAnnouncementListModel{}
	}
	res = new(appannouncement.ListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cAppAnnouncement) View(ctx context.Context, req *appannouncement.ViewReq) (res *appannouncement.ViewRes, err error) {
	data, err := service.SysAppAnnouncement().View(ctx, &req.AppAnnouncementViewInp)
	if err != nil {
		return
	}
	res = new(appannouncement.ViewRes)
	res.AppAnnouncementViewModel = data
	return
}

func (c *cAppAnnouncement) Edit(ctx context.Context, req *appannouncement.EditReq) (res *appannouncement.EditRes, err error) {
	err = service.SysAppAnnouncement().Edit(ctx, &req.AppAnnouncementEditInp)
	return
}

func (c *cAppAnnouncement) Delete(ctx context.Context, req *appannouncement.DeleteReq) (res *appannouncement.DeleteRes, err error) {
	err = service.SysAppAnnouncement().Delete(ctx, &req.AppAnnouncementDeleteInp)
	return
}

func (c *cAppAnnouncement) Status(ctx context.Context, req *appannouncement.StatusReq) (res *appannouncement.StatusRes, err error) {
	err = service.SysAppAnnouncement().Status(ctx, &req.AppAnnouncementStatusInp)
	return
}

func (c *cAppAnnouncement) MaxSort(ctx context.Context, req *appannouncement.MaxSortReq) (res *appannouncement.MaxSortRes, err error) {
	data, err := service.SysAppAnnouncement().MaxSort(ctx, &req.AppAnnouncementMaxSortInp)
	if err != nil {
		return
	}
	res = new(appannouncement.MaxSortRes)
	res.AppAnnouncementMaxSortModel = data
	return
}
