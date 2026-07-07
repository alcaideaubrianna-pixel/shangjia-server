// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCycleRunLogDao is the data access object for the table hg_youban_publish_cycle_run_log.
type YoubanPublishCycleRunLogDao struct {
	table    string                          // table is the underlying table name of the DAO.
	group    string                          // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCycleRunLogColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler              // handlers for customized model modification.
}

// YoubanPublishCycleRunLogColumns defines and stores column names for the table hg_youban_publish_cycle_run_log.
type YoubanPublishCycleRunLogColumns struct {
	Id          string //
	RunId       string //
	PlanId      string //
	TenantId    string //
	AccountId   string //
	ProfileId   string //
	Level       string //
	Stage       string //
	Message     string //
	ContextJson string //
	CreatedAt   string //
}

// youbanPublishCycleRunLogColumns holds the columns for the table hg_youban_publish_cycle_run_log.
var youbanPublishCycleRunLogColumns = YoubanPublishCycleRunLogColumns{
	Id:          "id",
	RunId:       "run_id",
	PlanId:      "plan_id",
	TenantId:    "tenant_id",
	AccountId:   "account_id",
	ProfileId:   "profile_id",
	Level:       "level",
	Stage:       "stage",
	Message:     "message",
	ContextJson: "context_json",
	CreatedAt:   "created_at",
}

// NewYoubanPublishCycleRunLogDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCycleRunLogDao(handlers ...gdb.ModelHandler) *YoubanPublishCycleRunLogDao {
	return &YoubanPublishCycleRunLogDao{
		group:    "default",
		table:    "hg_youban_publish_cycle_run_log",
		columns:  youbanPublishCycleRunLogColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCycleRunLogDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCycleRunLogDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCycleRunLogDao) Columns() YoubanPublishCycleRunLogColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCycleRunLogDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCycleRunLogDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCycleRunLogDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
