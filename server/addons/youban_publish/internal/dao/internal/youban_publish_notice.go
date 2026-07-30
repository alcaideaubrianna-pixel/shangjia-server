// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishNoticeDao is the data access object for the table hg_youban_publish_notice.
type YoubanPublishNoticeDao struct {
	table    string                     // table is the underlying table name of the DAO.
	group    string                     // group is the database configuration group name of the current DAO.
	columns  YoubanPublishNoticeColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler         // handlers for customized model modification.
}

// YoubanPublishNoticeColumns defines and stores column names for the table hg_youban_publish_notice.
type YoubanPublishNoticeColumns struct {
	Id        string //
	Type      string //
	Title     string //
	Content   string //
	Tag       string //
	Receiver  string //
	Remark    string //
	Sort      string //
	Status    string //
	PublishAt string //
	ExpireAt  string //
	CreatedBy string //
	UpdatedBy string //
	DeletedBy string //
	CreatedAt string //
	UpdatedAt string //
	DeletedAt string //
}

// youbanPublishNoticeColumns holds the columns for the table hg_youban_publish_notice.
var youbanPublishNoticeColumns = YoubanPublishNoticeColumns{
	Id:        "id",
	Type:      "type",
	Title:     "title",
	Content:   "content",
	Tag:       "tag",
	Receiver:  "receiver",
	Remark:    "remark",
	Sort:      "sort",
	Status:    "status",
	PublishAt: "publish_at",
	ExpireAt:  "expire_at",
	CreatedBy: "created_by",
	UpdatedBy: "updated_by",
	DeletedBy: "deleted_by",
	CreatedAt: "created_at",
	UpdatedAt: "updated_at",
	DeletedAt: "deleted_at",
}

// NewYoubanPublishNoticeDao creates and returns a new DAO object for table data access.
func NewYoubanPublishNoticeDao(handlers ...gdb.ModelHandler) *YoubanPublishNoticeDao {
	return &YoubanPublishNoticeDao{
		group:    "default",
		table:    "hg_youban_publish_notice",
		columns:  youbanPublishNoticeColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishNoticeDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishNoticeDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishNoticeDao) Columns() YoubanPublishNoticeColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishNoticeDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishNoticeDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishNoticeDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
