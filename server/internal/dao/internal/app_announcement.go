package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type AppAnnouncementDao struct {
	table    string
	group    string
	columns  AppAnnouncementColumns
	handlers []gdb.ModelHandler
}

type AppAnnouncementColumns struct {
	Id           string
	Title        string
	Content      string
	CategoryCode string
	CategoryName string
	Summary      string
	IsBanner     string
	BannerImg    string
	BannerUrl    string
	PublishAt    string
	ExpireAt     string
	Sort         string
	Status       string
	CreatedBy    string
	UpdatedBy    string
	CreatedAt    string
	UpdatedAt    string
	DeletedAt    string
}

var appAnnouncementColumns = AppAnnouncementColumns{
	Id:           "id",
	Title:        "title",
	Content:      "content",
	CategoryCode: "category_code",
	CategoryName: "category_name",
	Summary:      "summary",
	IsBanner:     "is_banner",
	BannerImg:    "banner_img",
	BannerUrl:    "banner_url",
	PublishAt:    "publish_at",
	ExpireAt:     "expire_at",
	Sort:         "sort",
	Status:       "status",
	CreatedBy:    "created_by",
	UpdatedBy:    "updated_by",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
	DeletedAt:    "deleted_at",
}

func NewAppAnnouncementDao(handlers ...gdb.ModelHandler) *AppAnnouncementDao {
	return &AppAnnouncementDao{
		group:    "default",
		table:    "hg_app_announcement",
		columns:  appAnnouncementColumns,
		handlers: handlers,
	}
}

func (dao *AppAnnouncementDao) DB() gdb.DB {
	return g.DB(dao.group)
}

func (dao *AppAnnouncementDao) Table() string {
	return dao.table
}

func (dao *AppAnnouncementDao) Columns() AppAnnouncementColumns {
	return dao.columns
}

func (dao *AppAnnouncementDao) Group() string {
	return dao.group
}

func (dao *AppAnnouncementDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

func (dao *AppAnnouncementDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
