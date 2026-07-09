// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectEventLogDao is the data access object for the table hg_youban_publish_collect_event_log.
type YoubanPublishCollectEventLogDao struct {
	table    string                              // table is the underlying table name of the DAO.
	group    string                              // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectEventLogColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                  // handlers for customized model modification.
}

// YoubanPublishCollectEventLogColumns defines and stores column names for the table hg_youban_publish_collect_event_log.
type YoubanPublishCollectEventLogColumns struct {
	Id         string //
	TenantId   string //
	AccountId  string //
	EventId    string //
	DispatchId string //
	Stage      string //
	Status     string //
	Message    string //
	MetaText   string //
	CreatedAt  string //
}

// youbanPublishCollectEventLogColumns holds the columns for the table hg_youban_publish_collect_event_log.
var youbanPublishCollectEventLogColumns = YoubanPublishCollectEventLogColumns{
	Id:         "id",
	TenantId:   "tenant_id",
	AccountId:  "account_id",
	EventId:    "event_id",
	DispatchId: "dispatch_id",
	Stage:      "stage",
	Status:     "status",
	Message:    "message",
	MetaText:   "meta_text",
	CreatedAt:  "created_at",
}

// NewYoubanPublishCollectEventLogDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectEventLogDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectEventLogDao {
	return &YoubanPublishCollectEventLogDao{
		group:    "default",
		table:    "hg_youban_publish_collect_event_log",
		columns:  youbanPublishCollectEventLogColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectEventLogDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectEventLogDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectEventLogDao) Columns() YoubanPublishCollectEventLogColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectEventLogDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectEventLogDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectEventLogDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
