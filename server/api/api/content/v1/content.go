package v1

import (
	"hotgo/internal/model/input/form"
	"hotgo/internal/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

type ListProfilesReq struct {
	g.Meta `path:"/content/profiles" method:"get" tags:"内容资料" summary:"获取公开资料列表"`
	sysin.ContentProfileListInp
}

type ListProfilesRes struct {
	form.PageRes
	List []*sysin.ContentProfileListModel `json:"list" dc:"资料列表"`
}

type HomeProfileCardsReq struct {
	g.Meta `path:"/home/profile-cards" method:"get" tags:"首页" summary:"获取首页资料卡片"`
	sysin.HomeProfileCardsInp
}

type HomeProfileCardsRes struct {
	form.PageRes
	List []*sysin.ContentProfileListModel `json:"list" dc:"资料卡片列表"`
}

type FilterOptionsReq struct {
	g.Meta `path:"/content/filter/options" method:"get" tags:"内容资料" summary:"获取公开资料筛选选项"`
}

type FilterOptionsRes struct {
	*sysin.ContentProfileFilterOptionsModel
}

type ViewProfileReq struct {
	g.Meta `path:"/content/profile/view" method:"get" tags:"内容资料" summary:"获取公开资料详情"`
	sysin.ContentProfileViewInp
}

type ViewProfileRes struct {
	*sysin.ContentProfileViewModel
}

type ListAnnouncementsReq struct {
	g.Meta `path:"/content/announcements" method:"get" tags:"前台公告" summary:"获取前台公告列表"`
	sysin.AppAnnouncementPublicListInp
}

type ListAnnouncementsRes struct {
	form.PageRes
	List []*sysin.AppAnnouncementPublicListModel `json:"list" dc:"公告列表"`
}
