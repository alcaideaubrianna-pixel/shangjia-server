// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTaskDao is the data access object for the table hg_youban_publish_task.
type YoubanPublishTaskDao struct {
	table    string                   // table is the underlying table name of the DAO.
	group    string                   // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTaskColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler       // handlers for customized model modification.
}

// YoubanPublishTaskColumns defines and stores column names for the table hg_youban_publish_task.
type YoubanPublishTaskColumns struct {
	Id              string // 主键
	MerchantId      string // 商家ID
	AccountId       string // 账号ID
	ProfileId       string // 资料ID
	ClientRequestId string // 客户端幂等ID
	Title           string // 标题
	Province        string // 省份
	City            string // 城市
	PlainText       string // 正文
	MediaCount      string // 媒体数量
	TgPushEnabled   string // 是否推送TG
	TgStatus        string // TG状态
	Status          string // 任务状态
	ErrorMessage    string // 错误信息
	SubmittedAt     string // 提交时间
	PublishedAt     string // 发布时间
	CreatedBy       string // 创建人
	UpdatedBy       string // 更新人
	DeletedBy       string // 删除人
	CreatedAt       string // 创建时间
	UpdatedAt       string // 更新时间
	DeletedAt       string // 删除时间
}

// youbanPublishTaskColumns holds the columns for the table hg_youban_publish_task.
var youbanPublishTaskColumns = YoubanPublishTaskColumns{
	Id:              "id",
	MerchantId:      "merchant_id",
	AccountId:       "account_id",
	ProfileId:       "profile_id",
	ClientRequestId: "client_request_id",
	Title:           "title",
	Province:        "province",
	City:            "city",
	PlainText:       "plain_text",
	MediaCount:      "media_count",
	TgPushEnabled:   "tg_push_enabled",
	TgStatus:        "tg_status",
	Status:          "status",
	ErrorMessage:    "error_message",
	SubmittedAt:     "submitted_at",
	PublishedAt:     "published_at",
	CreatedBy:       "created_by",
	UpdatedBy:       "updated_by",
	DeletedBy:       "deleted_by",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
	DeletedAt:       "deleted_at",
}

// NewYoubanPublishTaskDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTaskDao(handlers ...gdb.ModelHandler) *YoubanPublishTaskDao {
	return &YoubanPublishTaskDao{
		group:    "default",
		table:    "hg_youban_publish_task",
		columns:  youbanPublishTaskColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTaskDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTaskDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTaskDao) Columns() YoubanPublishTaskColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTaskDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTaskDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

// Transaction wraps the transaction logic using function f.
// It rolls back the transaction and returns the error if function f returns a non-nil error.
// It commits the transaction and returns nil if function f returns nil.
//
// Note: Do not commit or roll back the transaction in function f,
// as it is automatically handled by this function.
func (dao *YoubanPublishTaskDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
