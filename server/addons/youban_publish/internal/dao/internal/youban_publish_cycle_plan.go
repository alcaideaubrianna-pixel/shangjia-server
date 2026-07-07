// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCyclePlanDao is the data access object for the table hg_youban_publish_cycle_plan.
type YoubanPublishCyclePlanDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCyclePlanColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanPublishCyclePlanColumns defines and stores column names for the table hg_youban_publish_cycle_plan.
type YoubanPublishCyclePlanColumns struct {
	Id               string //
	TenantId         string //
	AccountId        string //
	ProfileId        string //
	TaskId           string //
	Enabled          string //
	IntervalSeconds  string //
	PublishTime      string //
	NextRunAt        string //
	LastRunAt        string //
	LastRunId        string //
	Status           string //
	Source           string //
	LockedAt         string //
	LastErrorMessage string //
	CreatedAt        string //
	UpdatedAt        string //
	DeletedAt        string //
}

// youbanPublishCyclePlanColumns holds the columns for the table hg_youban_publish_cycle_plan.
var youbanPublishCyclePlanColumns = YoubanPublishCyclePlanColumns{
	Id:               "id",
	TenantId:         "tenant_id",
	AccountId:        "account_id",
	ProfileId:        "profile_id",
	TaskId:           "task_id",
	Enabled:          "enabled",
	IntervalSeconds:  "interval_seconds",
	PublishTime:      "publish_time",
	NextRunAt:        "next_run_at",
	LastRunAt:        "last_run_at",
	LastRunId:        "last_run_id",
	Status:           "status",
	Source:           "source",
	LockedAt:         "locked_at",
	LastErrorMessage: "last_error_message",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
	DeletedAt:        "deleted_at",
}

// NewYoubanPublishCyclePlanDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCyclePlanDao(handlers ...gdb.ModelHandler) *YoubanPublishCyclePlanDao {
	return &YoubanPublishCyclePlanDao{
		group:    "default",
		table:    "hg_youban_publish_cycle_plan",
		columns:  youbanPublishCyclePlanColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCyclePlanDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCyclePlanDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCyclePlanDao) Columns() YoubanPublishCyclePlanColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCyclePlanDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCyclePlanDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCyclePlanDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
