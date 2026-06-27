package sysin

import (
	"context"
	"hotgo/internal/consts"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/form"
	"hotgo/utility/validate"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type AppAnnouncementUpdateFields struct {
	Title        string      `json:"title" dc:"公告标题"`
	Content      string      `json:"content" dc:"公告内容"`
	CategoryCode string      `json:"categoryCode" dc:"文章分类编码"`
	CategoryName string      `json:"categoryName" dc:"文章分类名称"`
	Summary      string      `json:"summary" dc:"摘要"`
	IsBanner     int         `json:"isBanner" dc:"是否Banner"`
	BannerImg    string      `json:"bannerImg" dc:"Banner图片"`
	BannerUrl    string      `json:"bannerUrl" dc:"Banner链接"`
	PublishAt    *gtime.Time `json:"publishAt" dc:"定时发布时间"`
	ExpireAt     *gtime.Time `json:"expireAt" dc:"过期时间"`
	Sort         int         `json:"sort" dc:"排序"`
	Status       int         `json:"status" dc:"状态"`
	UpdatedBy    int64       `json:"updatedBy" dc:"更新者"`
}

type AppAnnouncementInsertFields struct {
	Title        string      `json:"title" dc:"公告标题"`
	Content      string      `json:"content" dc:"公告内容"`
	CategoryCode string      `json:"categoryCode" dc:"文章分类编码"`
	CategoryName string      `json:"categoryName" dc:"文章分类名称"`
	Summary      string      `json:"summary" dc:"摘要"`
	IsBanner     int         `json:"isBanner" dc:"是否Banner"`
	BannerImg    string      `json:"bannerImg" dc:"Banner图片"`
	BannerUrl    string      `json:"bannerUrl" dc:"Banner链接"`
	PublishAt    *gtime.Time `json:"publishAt" dc:"定时发布时间"`
	ExpireAt     *gtime.Time `json:"expireAt" dc:"过期时间"`
	Sort         int         `json:"sort" dc:"排序"`
	Status       int         `json:"status" dc:"状态"`
	CreatedBy    int64       `json:"createdBy" dc:"创建者"`
}

type AppAnnouncementEditInp struct {
	entity.AppAnnouncement
}

func (in *AppAnnouncementEditInp) Filter(ctx context.Context) (err error) {
	if e := g.Validator().Rules("required").Data(in.Title).Messages("公告标题不能为空").Run(ctx); e != nil {
		return e.Current()
	}
	if e := g.Validator().Rules("required").Data(in.Content).Messages("公告内容不能为空").Run(ctx); e != nil {
		return e.Current()
	}
	if in.Status <= 0 {
		in.Status = consts.StatusEnabled
	}
	if in.CategoryCode == "" {
		in.CategoryCode = "blog"
	}
	if in.CategoryName == "" {
		in.CategoryName = articleCategoryName(in.CategoryCode)
	}
	if !validate.InSlice(consts.StatusSlice, in.Status) {
		return gerror.New("公告状态不正确")
	}
	return nil
}

type AppAnnouncementListInp struct {
	form.PageReq
	Title        string        `json:"title" dc:"公告标题"`
	CategoryCode string        `json:"categoryCode" dc:"文章分类编码"`
	IsBanner     int           `json:"isBanner" dc:"是否Banner"`
	Status       int           `json:"status" dc:"状态"`
	CreatedAt    []*gtime.Time `json:"createdAt" dc:"创建时间"`
}

type AppAnnouncementListModel struct {
	entity.AppAnnouncement
}

type AppAnnouncementViewInp struct {
	Id int64 `json:"id" v:"required#ID不能为空" dc:"ID"`
}

type AppAnnouncementViewModel struct {
	entity.AppAnnouncement
}

type AppAnnouncementDeleteInp struct {
	Id interface{} `json:"id" v:"required#ID不能为空" dc:"ID"`
}

type AppAnnouncementStatusInp struct {
	Id     int64 `json:"id" v:"required#ID不能为空" dc:"ID"`
	Status int   `json:"status" dc:"状态"`
}

func (in *AppAnnouncementStatusInp) Filter(ctx context.Context) (err error) {
	if in.Status <= 0 {
		return gerror.New("状态不能为空")
	}
	if !validate.InSlice(consts.StatusSlice, in.Status) {
		return gerror.New("状态不正确")
	}
	return nil
}

type AppAnnouncementMaxSortInp struct{}

type AppAnnouncementMaxSortModel struct {
	Sort int `json:"sort" dc:"排序"`
}

type AppAnnouncementPublicListInp struct {
	form.PageReq
	IsBanner     int    `json:"isBanner" dc:"是否Banner"`
	CategoryCode string `json:"categoryCode" dc:"文章分类编码"`
}

type AppAnnouncementCategoryModel struct {
	Code        string `json:"code" dc:"分类编码"`
	Name        string `json:"name" dc:"分类名称"`
	Description string `json:"description" dc:"分类描述"`
	TotalCount  int    `json:"totalCount" dc:"文章数量"`
}

type SeoFooterLinkModel struct {
	Label string `json:"label" dc:"链接文本"`
	Url   string `json:"url" dc:"链接地址"`
	Count int    `json:"count" dc:"数量"`
}

type SeoFooterGroupModel struct {
	Title string                `json:"title" dc:"分组标题"`
	Links []*SeoFooterLinkModel `json:"links" dc:"链接列表"`
}

type SeoFooterModel struct {
	Groups      []*SeoFooterGroupModel `json:"groups" dc:"页脚分组"`
	BrandTitle  string                 `json:"brandTitle" dc:"品牌标题"`
	Notice      string                 `json:"notice" dc:"合规提示"`
	Copyright   string                 `json:"copyright" dc:"版权"`
	PolicyLinks []*SeoFooterLinkModel  `json:"policyLinks" dc:"政策链接"`
}

type AppAnnouncementPublicViewInp struct {
	Id int64 `json:"id" v:"required#公告ID不能为空" dc:"公告ID"`
}

type AppAnnouncementPublicListModel struct {
	Id           int64       `json:"id" dc:"ID"`
	Title        string      `json:"title" dc:"公告标题"`
	Content      string      `json:"content" dc:"公告内容"`
	CategoryCode string      `json:"categoryCode" dc:"文章分类编码"`
	CategoryName string      `json:"categoryName" dc:"文章分类名称"`
	Summary      string      `json:"summary" dc:"摘要"`
	IsBanner     int         `json:"isBanner" dc:"是否Banner"`
	BannerImg    string      `json:"bannerImg" dc:"Banner图片"`
	BannerUrl    string      `json:"bannerUrl" dc:"Banner链接"`
	Status       int         `json:"status" dc:"状态"`
	PublishAt    *gtime.Time `json:"publishAt" dc:"发布时间"`
	ExpireAt     *gtime.Time `json:"expireAt" dc:"过期时间"`
	CreatedAt    *gtime.Time `json:"createdAt" dc:"创建时间"`
}

func ArticleCategories() []*AppAnnouncementCategoryModel {
	return []*AppAnnouncementCategoryModel{
		{Code: "about", Name: "关于我们", Description: "了解悦伴平台、服务规则和用户权益。"},
		{Code: "case", Name: "交友成功案例", Description: "分享真实同城交友体验和平台使用案例。"},
		{Code: "blog", Name: "博客", Description: "分享同城交友、高端社交和会员认证相关看法。"},
		{Code: "docs", Name: "文档", Description: "查看平台使用说明、隐私保护和会员权益说明。"},
		{Code: "news", Name: "新闻", Description: "了解悦伴最新公告、城市频道和功能更新。"},
	}
}

func articleCategoryName(code string) string {
	for _, item := range ArticleCategories() {
		if item.Code == code {
			return item.Name
		}
	}
	return "博客"
}
