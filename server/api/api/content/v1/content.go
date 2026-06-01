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

type ViewProfileReq struct {
	g.Meta `path:"/content/profile/view" method:"get" tags:"内容资料" summary:"获取公开资料详情"`
	sysin.ContentProfileViewInp
}

type ViewProfileRes struct {
	*sysin.ContentProfileViewModel
}
