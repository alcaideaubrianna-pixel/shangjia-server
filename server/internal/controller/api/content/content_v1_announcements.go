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
