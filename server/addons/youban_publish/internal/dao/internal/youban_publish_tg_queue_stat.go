// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgQueueStatDao is the data access object for the table hg_youban_publish_tg_queue_stat.
type YoubanPublishTgQueueStatDao struct {
	table    string                          // table is the underlying table name of the DAO.
	group    string                          // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgQueueStatColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler              // handlers for customized model modification.
}

// YoubanPublishTgQueueStatColumns defines and stores column names for the table hg_youban_publish_tg_queue_stat.
type YoubanPublishTgQueueStatColumns struct {
	Id            string //
	StatTime      string //
	QueueName     string //
	PriorityLevel string //
	Status        string //
	JobCount      string //
	OldestJobAt   string //
	LatestJobAt   string //
	CreatedAt     string //
	UpdatedAt     string //
}

// youbanPublishTgQueueStatColumns holds the columns for the table hg_youban_publish_tg_queue_stat.
var youbanPublishTgQueueStatColumns = YoubanPublishTgQueueStatColumns{
	Id:            "id",
	StatTime:      "stat_time",
	QueueName:     "queue_name",
	PriorityLevel: "priority_level",
	Status:        "status",
	JobCount:      "job_count",
	OldestJobAt:   "oldest_job_at",
	LatestJobAt:   "latest_job_at",
	CreatedAt:     "created_at",
	UpdatedAt:     "updated_at",
}

// NewYoubanPublishTgQueueStatDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgQueueStatDao(handlers ...gdb.ModelHandler) *YoubanPublishTgQueueStatDao {
	return &YoubanPublishTgQueueStatDao{
		group:    "default",
		table:    "hg_youban_publish_tg_queue_stat",
		columns:  youbanPublishTgQueueStatColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgQueueStatDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgQueueStatDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgQueueStatDao) Columns() YoubanPublishTgQueueStatColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgQueueStatDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgQueueStatDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgQueueStatDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
