// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotRelayAdminDao is the data access object for the table hg_youban_bot_relay_admin.
type YoubanBotRelayAdminDao struct {
	table    string                     // table is the underlying table name of the DAO.
	group    string                     // group is the database configuration group name of the current DAO.
	columns  YoubanBotRelayAdminColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler         // handlers for customized model modification.
}

// YoubanBotRelayAdminColumns defines and stores column names for the table hg_youban_bot_relay_admin.
type YoubanBotRelayAdminColumns struct {
	Id        string //
	BotId     string //
	UserId    string //
	Username  string //
	CreatedAt string //
	UpdatedAt string //
}

// youbanBotRelayAdminColumns holds the columns for the table hg_youban_bot_relay_admin.
var youbanBotRelayAdminColumns = YoubanBotRelayAdminColumns{
	Id:        "id",
	BotId:     "bot_id",
	UserId:    "user_id",
	Username:  "username",
	CreatedAt: "created_at",
	UpdatedAt: "updated_at",
}

// NewYoubanBotRelayAdminDao creates and returns a new DAO object for table data access.
func NewYoubanBotRelayAdminDao(handlers ...gdb.ModelHandler) *YoubanBotRelayAdminDao {
	return &YoubanBotRelayAdminDao{
		group:    "default",
		table:    "hg_youban_bot_relay_admin",
		columns:  youbanBotRelayAdminColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotRelayAdminDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotRelayAdminDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotRelayAdminDao) Columns() YoubanBotRelayAdminColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotRelayAdminDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotRelayAdminDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotRelayAdminDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
