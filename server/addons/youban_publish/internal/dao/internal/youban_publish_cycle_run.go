// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCycleRunDao is the data access object for the table hg_youban_publish_cycle_run.
type YoubanPublishCycleRunDao struct {
	table    string                       // table is the underlying table name of the DAO.
	group    string                       // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCycleRunColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler           // handlers for customized model modification.
}

// YoubanPublishCycleRunColumns defines and stores column names for the table hg_youban_publish_cycle_run.
type YoubanPublishCycleRunColumns struct {
	Id           string //
	PlanId       string //
	TenantId     string //
	AccountId    string //
	ProfileId    string //
	TaskId       string //
	Status       string //
	Stage        string //
	ScheduledAt  string //
	StartedAt    string //
	FinishedAt   string //
	ErrorMessage string //
	RetryCount   string //
	CreatedAt    string //
	UpdatedAt    string //
}

// youbanPublishCycleRunColumns holds the columns for the table hg_youban_publish_cycle_run.
var youbanPublishCycleRunColumns = YoubanPublishCycleRunColumns{
	Id:           "id",
	PlanId:       "plan_id",
	TenantId:     "tenant_id",
	AccountId:    "account_id",
	ProfileId:    "profile_id",
	TaskId:       "task_id",
	Status:       "status",
	Stage:        "stage",
	ScheduledAt:  "scheduled_at",
	StartedAt:    "started_at",
	FinishedAt:   "finished_at",
	ErrorMessage: "error_message",
	RetryCount:   "retry_count",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
}

// NewYoubanPublishCycleRunDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCycleRunDao(handlers ...gdb.ModelHandler) *YoubanPublishCycleRunDao {
	return &YoubanPublishCycleRunDao{
		group:    "default",
		table:    "hg_youban_publish_cycle_run",
		columns:  youbanPublishCycleRunColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCycleRunDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCycleRunDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCycleRunDao) Columns() YoubanPublishCycleRunColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCycleRunDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCycleRunDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCycleRunDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
