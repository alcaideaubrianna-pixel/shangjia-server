// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotRelayBindingDao is the data access object for the table hg_youban_bot_relay_binding.
type YoubanBotRelayBindingDao struct {
	table    string                       // table is the underlying table name of the DAO.
	group    string                       // group is the database configuration group name of the current DAO.
	columns  YoubanBotRelayBindingColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler           // handlers for customized model modification.
}

// YoubanBotRelayBindingColumns defines and stores column names for the table hg_youban_bot_relay_binding.
type YoubanBotRelayBindingColumns struct {
	Id          string //
	BotId       string //
	PairKey     string //
	LeftChatId  string //
	RightChatId string //
	LeftName    string //
	RightName   string //
	CreatedAt   string //
	UpdatedAt   string //
}

// youbanBotRelayBindingColumns holds the columns for the table hg_youban_bot_relay_binding.
var youbanBotRelayBindingColumns = YoubanBotRelayBindingColumns{
	Id:          "id",
	BotId:       "bot_id",
	PairKey:     "pair_key",
	LeftChatId:  "left_chat_id",
	RightChatId: "right_chat_id",
	LeftName:    "left_name",
	RightName:   "right_name",
	CreatedAt:   "created_at",
	UpdatedAt:   "updated_at",
}

// NewYoubanBotRelayBindingDao creates and returns a new DAO object for table data access.
func NewYoubanBotRelayBindingDao(handlers ...gdb.ModelHandler) *YoubanBotRelayBindingDao {
	return &YoubanBotRelayBindingDao{
		group:    "default",
		table:    "hg_youban_bot_relay_binding",
		columns:  youbanBotRelayBindingColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotRelayBindingDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotRelayBindingDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotRelayBindingDao) Columns() YoubanBotRelayBindingColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotRelayBindingDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotRelayBindingDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotRelayBindingDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
