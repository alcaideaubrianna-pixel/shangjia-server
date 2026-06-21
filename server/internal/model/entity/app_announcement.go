package entity

import "github.com/gogf/gf/v2/os/gtime"

type AppAnnouncement struct {
	Id        int64       `json:"id"        orm:"id"         description:"ID"`
	Title     string      `json:"title"     orm:"title"      description:"公告标题"`
	Content   string      `json:"content"   orm:"content"    description:"公告内容"`
	IsBanner  int         `json:"isBanner"  orm:"is_banner"  description:"是否Banner"`
	BannerImg string      `json:"bannerImg" orm:"banner_img" description:"Banner图片"`
	BannerUrl string      `json:"bannerUrl" orm:"banner_url" description:"Banner链接"`
	PublishAt *gtime.Time `json:"publishAt" orm:"publish_at" description:"定时发布时间"`
	ExpireAt  *gtime.Time `json:"expireAt"  orm:"expire_at"  description:"过期时间"`
	Sort      int         `json:"sort"      orm:"sort"       description:"排序"`
	Status    int         `json:"status"    orm:"status"     description:"状态"`
	CreatedBy int64       `json:"createdBy" orm:"created_by" description:"创建者"`
	UpdatedBy int64       `json:"updatedBy" orm:"updated_by" description:"更新者"`
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" description:"创建时间"`
	UpdatedAt *gtime.Time `json:"updatedAt" orm:"updated_at" description:"更新时间"`
	DeletedAt *gtime.Time `json:"deletedAt" orm:"deleted_at" description:"删除时间"`
}
