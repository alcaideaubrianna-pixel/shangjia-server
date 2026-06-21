package appannouncement

import (
	"hotgo/internal/model/input/form"
	"hotgo/internal/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

type ListReq struct {
	g.Meta `path:"/appAnnouncement/list" method:"get" tags:"APP公告" summary:"获取APP公告列表"`
	sysin.AppAnnouncementListInp
}

type ListRes struct {
	form.PageRes
	List []*sysin.AppAnnouncementListModel `json:"list" dc:"数据列表"`
}

type ViewReq struct {
	g.Meta `path:"/appAnnouncement/view" method:"get" tags:"APP公告" summary:"获取APP公告详情"`
	sysin.AppAnnouncementViewInp
}

type ViewRes struct {
	*sysin.AppAnnouncementViewModel
}

type EditReq struct {
	g.Meta `path:"/appAnnouncement/edit" method:"post" tags:"APP公告" summary:"修改/新增APP公告"`
	sysin.AppAnnouncementEditInp
}

type EditRes struct{}

type DeleteReq struct {
	g.Meta `path:"/appAnnouncement/delete" method:"post" tags:"APP公告" summary:"删除APP公告"`
	sysin.AppAnnouncementDeleteInp
}

type DeleteRes struct{}

type StatusReq struct {
	g.Meta `path:"/appAnnouncement/status" method:"post" tags:"APP公告" summary:"更新APP公告状态"`
	sysin.AppAnnouncementStatusInp
}

type StatusRes struct{}

type MaxSortReq struct {
	g.Meta `path:"/appAnnouncement/maxSort" method:"get" tags:"APP公告" summary:"获取最大排序"`
	sysin.AppAnnouncementMaxSortInp
}

type MaxSortRes struct {
	*sysin.AppAnnouncementMaxSortModel
}
