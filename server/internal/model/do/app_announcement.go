package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type AppAnnouncement struct {
	g.Meta       `orm:"table:hg_app_announcement, do:true"`
	Id           any
	Title        any
	Content      any
	CategoryCode any
	CategoryName any
	Summary      any
	IsBanner     any
	BannerImg    any
	BannerUrl    any
	PublishAt    *gtime.Time
	ExpireAt     *gtime.Time
	Sort         any
	Status       any
	CreatedBy    any
	UpdatedBy    any
	CreatedAt    *gtime.Time
	UpdatedAt    *gtime.Time
	DeletedAt    *gtime.Time
}
