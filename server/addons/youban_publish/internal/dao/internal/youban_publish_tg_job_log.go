// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgJobLogDao is the data access object for the table hg_youban_publish_tg_job_log.
type YoubanPublishTgJobLogDao struct {
	table    string                       // table is the underlying table name of the DAO.
	group    string                       // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgJobLogColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler           // handlers for customized model modification.
}

// YoubanPublishTgJobLogColumns defines and stores column names for the table hg_youban_publish_tg_job_log.
type YoubanPublishTgJobLogColumns struct {
	Id        string // 主键
	JobId     string // TG任务ID
	TaskId    string // 任务ID
	TenantId  string // 租户ID
	AccountId string // 账号ID
	ProfileId string // 资料ID
	BotId     string // Bot ID
	Action    string // 动作
	Status    string // 状态
	Message   string // 日志内容
	CreatedAt string // 创建时间
}

// youbanPublishTgJobLogColumns holds the columns for the table hg_youban_publish_tg_job_log.
var youbanPublishTgJobLogColumns = YoubanPublishTgJobLogColumns{
	Id:        "id",
	JobId:     "job_id",
	TaskId:    "task_id",
	TenantId:  "tenant_id",
	AccountId: "account_id",
	ProfileId: "profile_id",
	BotId:     "bot_id",
	Action:    "action",
	Status:    "status",
	Message:   "message",
	CreatedAt: "created_at",
}

// NewYoubanPublishTgJobLogDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgJobLogDao(handlers ...gdb.ModelHandler) *YoubanPublishTgJobLogDao {
	return &YoubanPublishTgJobLogDao{
		group:    "default",
		table:    "hg_youban_publish_tg_job_log",
		columns:  youbanPublishTgJobLogColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgJobLogDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgJobLogDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgJobLogDao) Columns() YoubanPublishTgJobLogColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgJobLogDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgJobLogDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgJobLogDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
