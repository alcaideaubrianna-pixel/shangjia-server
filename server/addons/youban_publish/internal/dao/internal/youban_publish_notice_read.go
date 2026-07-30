// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishNoticeReadDao is the data access object for the table hg_youban_publish_notice_read.
type YoubanPublishNoticeReadDao struct {
	table    string                         // table is the underlying table name of the DAO.
	group    string                         // group is the database configuration group name of the current DAO.
	columns  YoubanPublishNoticeReadColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler             // handlers for customized model modification.
}

// YoubanPublishNoticeReadColumns defines and stores column names for the table hg_youban_publish_notice_read.
type YoubanPublishNoticeReadColumns struct {
	Id        string //
	NoticeId  string //
	AccountId string //
	Clicks    string //
	CreatedAt string //
	UpdatedAt string //
}

// youbanPublishNoticeReadColumns holds the columns for the table hg_youban_publish_notice_read.
var youbanPublishNoticeReadColumns = YoubanPublishNoticeReadColumns{
	Id:        "id",
	NoticeId:  "notice_id",
	AccountId: "account_id",
	Clicks:    "clicks",
	CreatedAt: "created_at",
	UpdatedAt: "updated_at",
}

// NewYoubanPublishNoticeReadDao creates and returns a new DAO object for table data access.
func NewYoubanPublishNoticeReadDao(handlers ...gdb.ModelHandler) *YoubanPublishNoticeReadDao {
	return &YoubanPublishNoticeReadDao{
		group:    "default",
		table:    "hg_youban_publish_notice_read",
		columns:  youbanPublishNoticeReadColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishNoticeReadDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishNoticeReadDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishNoticeReadDao) Columns() YoubanPublishNoticeReadColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishNoticeReadDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishNoticeReadDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishNoticeReadDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
