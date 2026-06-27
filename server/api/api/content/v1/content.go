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

type ImageSearchReq struct {
	g.Meta `path:"/content/profile/image-search" method:"post" mime:"multipart/form-data" tags:"内容资料" summary:"上传图片搜索相似资料"`
	sysin.ContentProfileImageSearchInp
}

type ImageSearchRes struct {
	form.PageRes
	List []*sysin.ContentProfileListModel `json:"list" dc:"相似资料列表"`
}

type FilterOptionsReq struct {
	g.Meta `path:"/content/filter/options" method:"get" tags:"内容资料" summary:"获取公开资料筛选选项"`
}

type FilterOptionsRes struct {
	*sysin.ContentProfileFilterOptionsModel
}

type RegionsReq struct {
	g.Meta `path:"/content/regions" method:"get" tags:"内容资料" summary:"获取公开地区目录"`
}

type RegionsRes struct {
	*sysin.ContentProfileRegionsModel
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

type ViewAnnouncementReq struct {
	g.Meta `path:"/content/announcement/view" method:"get" tags:"前台公告" summary:"获取前台公告详情"`
	sysin.AppAnnouncementPublicViewInp
}

type ViewAnnouncementRes struct {
	*sysin.AppAnnouncementPublicListModel
}

type ListArticleCategoriesReq struct {
	g.Meta `path:"/content/article/categories" method:"get" tags:"前台文章" summary:"获取文章分类列表"`
}

type ListArticleCategoriesRes struct {
	List []*sysin.AppAnnouncementCategoryModel `json:"list" dc:"分类列表"`
}

type SeoFooterReq struct {
	g.Meta `path:"/content/seo-footer" method:"get" tags:"前台SEO" summary:"获取SEO页脚配置"`
}

type SeoFooterRes struct {
	*sysin.SeoFooterModel
}
