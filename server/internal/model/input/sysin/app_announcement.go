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
	Title     string      `json:"title" dc:"公告标题"`
	Content   string      `json:"content" dc:"公告内容"`
	IsBanner  int         `json:"isBanner" dc:"是否Banner"`
	BannerImg string      `json:"bannerImg" dc:"Banner图片"`
	BannerUrl string      `json:"bannerUrl" dc:"Banner链接"`
	PublishAt *gtime.Time `json:"publishAt" dc:"定时发布时间"`
	ExpireAt  *gtime.Time `json:"expireAt" dc:"过期时间"`
	Sort      int         `json:"sort" dc:"排序"`
	Status    int         `json:"status" dc:"状态"`
	UpdatedBy int64       `json:"updatedBy" dc:"更新者"`
}

type AppAnnouncementInsertFields struct {
	Title     string      `json:"title" dc:"公告标题"`
	Content   string      `json:"content" dc:"公告内容"`
	IsBanner  int         `json:"isBanner" dc:"是否Banner"`
	BannerImg string      `json:"bannerImg" dc:"Banner图片"`
	BannerUrl string      `json:"bannerUrl" dc:"Banner链接"`
	PublishAt *gtime.Time `json:"publishAt" dc:"定时发布时间"`
	ExpireAt  *gtime.Time `json:"expireAt" dc:"过期时间"`
	Sort      int         `json:"sort" dc:"排序"`
	Status    int         `json:"status" dc:"状态"`
	CreatedBy int64       `json:"createdBy" dc:"创建者"`
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
	if !validate.InSlice(consts.StatusSlice, in.Status) {
		return gerror.New("公告状态不正确")
	}
	return nil
}

type AppAnnouncementListInp struct {
	form.PageReq
	Title     string        `json:"title" dc:"公告标题"`
	IsBanner  int           `json:"isBanner" dc:"是否Banner"`
	Status    int           `json:"status" dc:"状态"`
	CreatedAt []*gtime.Time `json:"createdAt" dc:"创建时间"`
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
	IsBanner int `json:"isBanner" dc:"是否Banner"`
}

type AppAnnouncementPublicListModel struct {
	Id        int64       `json:"id" dc:"ID"`
	Title     string      `json:"title" dc:"公告标题"`
	Content   string      `json:"content" dc:"公告内容"`
	IsBanner  int         `json:"isBanner" dc:"是否Banner"`
	BannerImg string      `json:"bannerImg" dc:"Banner图片"`
	BannerUrl string      `json:"bannerUrl" dc:"Banner链接"`
	Status    int         `json:"status" dc:"状态"`
	PublishAt *gtime.Time `json:"publishAt" dc:"发布时间"`
	ExpireAt  *gtime.Time `json:"expireAt" dc:"过期时间"`
	CreatedAt *gtime.Time `json:"createdAt" dc:"创建时间"`
}
