// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectHistoryLogDao is the data access object for the table hg_youban_publish_collect_history_log.
type YoubanPublishCollectHistoryLogDao struct {
	table    string                                // table is the underlying table name of the DAO.
	group    string                                // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectHistoryLogColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                    // handlers for customized model modification.
}

// YoubanPublishCollectHistoryLogColumns defines and stores column names for the table hg_youban_publish_collect_history_log.
type YoubanPublishCollectHistoryLogColumns struct {
	Id        string //
	TaskId    string //
	TenantId  string //
	AccountId string //
	Level     string //
	Stage     string //
	Message   string //
	MetaJson  string //
	CreatedAt string //
}

// youbanPublishCollectHistoryLogColumns holds the columns for the table hg_youban_publish_collect_history_log.
var youbanPublishCollectHistoryLogColumns = YoubanPublishCollectHistoryLogColumns{
	Id:        "id",
	TaskId:    "task_id",
	TenantId:  "tenant_id",
	AccountId: "account_id",
	Level:     "level",
	Stage:     "stage",
	Message:   "message",
	MetaJson:  "meta_json",
	CreatedAt: "created_at",
}

// NewYoubanPublishCollectHistoryLogDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectHistoryLogDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectHistoryLogDao {
	return &YoubanPublishCollectHistoryLogDao{
		group:    "default",
		table:    "hg_youban_publish_collect_history_log",
		columns:  youbanPublishCollectHistoryLogColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectHistoryLogDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectHistoryLogDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectHistoryLogDao) Columns() YoubanPublishCollectHistoryLogColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectHistoryLogDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectHistoryLogDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectHistoryLogDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
