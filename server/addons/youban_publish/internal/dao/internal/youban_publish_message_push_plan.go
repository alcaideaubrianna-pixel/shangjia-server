// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishMessagePushPlanDao is the data access object for the table hg_youban_publish_message_push_plan.
type YoubanPublishMessagePushPlanDao struct {
	table    string                              // table is the underlying table name of the DAO.
	group    string                              // group is the database configuration group name of the current DAO.
	columns  YoubanPublishMessagePushPlanColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                  // handlers for customized model modification.
}

// YoubanPublishMessagePushPlanColumns defines and stores column names for the table hg_youban_publish_message_push_plan.
type YoubanPublishMessagePushPlanColumns struct {
	Id              string //
	TenantId        string //
	Name            string //
	AccountId       string //
	TemplateIds     string //
	TargetChatIds   string //
	Times           string //
	IntervalSeconds string //
	Status          string //
	NextRunAt       string //
	LastRunAt       string //
	LastResult      string //
	LockedAt        string //
	CreatedBy       string //
	UpdatedBy       string //
	DeletedBy       string //
	CreatedAt       string //
	UpdatedAt       string //
	DeletedAt       string //
}

// youbanPublishMessagePushPlanColumns holds the columns for the table hg_youban_publish_message_push_plan.
var youbanPublishMessagePushPlanColumns = YoubanPublishMessagePushPlanColumns{
	Id:              "id",
	TenantId:        "tenant_id",
	Name:            "name",
	AccountId:       "account_id",
	TemplateIds:     "template_ids",
	TargetChatIds:   "target_chat_ids",
	Times:           "times",
	IntervalSeconds: "interval_seconds",
	Status:          "status",
	NextRunAt:       "next_run_at",
	LastRunAt:       "last_run_at",
	LastResult:      "last_result",
	LockedAt:        "locked_at",
	CreatedBy:       "created_by",
	UpdatedBy:       "updated_by",
	DeletedBy:       "deleted_by",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
	DeletedAt:       "deleted_at",
}

// NewYoubanPublishMessagePushPlanDao creates and returns a new DAO object for table data access.
func NewYoubanPublishMessagePushPlanDao(handlers ...gdb.ModelHandler) *YoubanPublishMessagePushPlanDao {
	return &YoubanPublishMessagePushPlanDao{
		group:    "default",
		table:    "hg_youban_publish_message_push_plan",
		columns:  youbanPublishMessagePushPlanColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishMessagePushPlanDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishMessagePushPlanDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishMessagePushPlanDao) Columns() YoubanPublishMessagePushPlanColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishMessagePushPlanDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishMessagePushPlanDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishMessagePushPlanDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
