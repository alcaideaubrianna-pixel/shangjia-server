// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishAccountNoticeDao is the data access object for the table hg_youban_publish_account_notice.
type YoubanPublishAccountNoticeDao struct {
	table    string                            // table is the underlying table name of the DAO.
	group    string                            // group is the database configuration group name of the current DAO.
	columns  YoubanPublishAccountNoticeColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                // handlers for customized model modification.
}

// YoubanPublishAccountNoticeColumns defines and stores column names for the table hg_youban_publish_account_notice.
type YoubanPublishAccountNoticeColumns struct {
	Id           string //
	TenantId     string //
	AccountId    string //
	EventKey     string //
	Category     string //
	TextContent  string //
	Status       string //
	Attempts     string //
	NextRetryAt  string //
	LeaseUntil   string //
	LeaseToken   string //
	ErrorMessage string //
	CreatedAt    string //
	UpdatedAt    string //
}

// youbanPublishAccountNoticeColumns holds the columns for the table hg_youban_publish_account_notice.
var youbanPublishAccountNoticeColumns = YoubanPublishAccountNoticeColumns{
	Id:           "id",
	TenantId:     "tenant_id",
	AccountId:    "account_id",
	EventKey:     "event_key",
	Category:     "category",
	TextContent:  "text_content",
	Status:       "status",
	Attempts:     "attempts",
	NextRetryAt:  "next_retry_at",
	LeaseUntil:   "lease_until",
	LeaseToken:   "lease_token",
	ErrorMessage: "error_message",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
}

// NewYoubanPublishAccountNoticeDao creates and returns a new DAO object for table data access.
func NewYoubanPublishAccountNoticeDao(handlers ...gdb.ModelHandler) *YoubanPublishAccountNoticeDao {
	return &YoubanPublishAccountNoticeDao{
		group:    "default",
		table:    "hg_youban_publish_account_notice",
		columns:  youbanPublishAccountNoticeColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishAccountNoticeDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishAccountNoticeDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishAccountNoticeDao) Columns() YoubanPublishAccountNoticeColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishAccountNoticeDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishAccountNoticeDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishAccountNoticeDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
