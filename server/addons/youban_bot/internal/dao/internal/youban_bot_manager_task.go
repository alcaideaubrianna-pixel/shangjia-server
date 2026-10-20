// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotManagerTaskDao is the data access object for the table hg_youban_bot_manager_task.
type YoubanBotManagerTaskDao struct {
	table    string                      // table is the underlying table name of the DAO.
	group    string                      // group is the database configuration group name of the current DAO.
	columns  YoubanBotManagerTaskColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler          // handlers for customized model modification.
}

// YoubanBotManagerTaskColumns defines and stores column names for the table hg_youban_bot_manager_task.
type YoubanBotManagerTaskColumns struct {
	Id             string //
	TenantId       string //
	AccountId      string //
	EntryBotId     string //
	TelegramUserId string //
	BatchKey       string //
	Action         string //
	ManagedBotId   string //
	ChannelId      string //
	RightsJson     string //
	Status         string //
	AccountTaskId  string //
	ErrorMessage   string //
	Notified       string //
	CreatedAt      string //
	UpdatedAt      string //
}

// youbanBotManagerTaskColumns holds the columns for the table hg_youban_bot_manager_task.
var youbanBotManagerTaskColumns = YoubanBotManagerTaskColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	AccountId:      "account_id",
	EntryBotId:     "entry_bot_id",
	TelegramUserId: "telegram_user_id",
	BatchKey:       "batch_key",
	Action:         "action",
	ManagedBotId:   "managed_bot_id",
	ChannelId:      "channel_id",
	RightsJson:     "rights_json",
	Status:         "status",
	AccountTaskId:  "account_task_id",
	ErrorMessage:   "error_message",
	Notified:       "notified",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
}

// NewYoubanBotManagerTaskDao creates and returns a new DAO object for table data access.
func NewYoubanBotManagerTaskDao(handlers ...gdb.ModelHandler) *YoubanBotManagerTaskDao {
	return &YoubanBotManagerTaskDao{
		group:    "default",
		table:    "hg_youban_bot_manager_task",
		columns:  youbanBotManagerTaskColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotManagerTaskDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotManagerTaskDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotManagerTaskDao) Columns() YoubanBotManagerTaskColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotManagerTaskDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotManagerTaskDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotManagerTaskDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
