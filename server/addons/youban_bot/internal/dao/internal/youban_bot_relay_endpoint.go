// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotRelayEndpointDao is the data access object for the table hg_youban_bot_relay_endpoint.
type YoubanBotRelayEndpointDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanBotRelayEndpointColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanBotRelayEndpointColumns defines and stores column names for the table hg_youban_bot_relay_endpoint.
type YoubanBotRelayEndpointColumns struct {
	Id        string //
	BotId     string //
	BindingId string //
	ChatId    string //
	CreatedAt string //
	UpdatedAt string //
}

// youbanBotRelayEndpointColumns holds the columns for the table hg_youban_bot_relay_endpoint.
var youbanBotRelayEndpointColumns = YoubanBotRelayEndpointColumns{
	Id:        "id",
	BotId:     "bot_id",
	BindingId: "binding_id",
	ChatId:    "chat_id",
	CreatedAt: "created_at",
	UpdatedAt: "updated_at",
}

// NewYoubanBotRelayEndpointDao creates and returns a new DAO object for table data access.
func NewYoubanBotRelayEndpointDao(handlers ...gdb.ModelHandler) *YoubanBotRelayEndpointDao {
	return &YoubanBotRelayEndpointDao{
		group:    "default",
		table:    "hg_youban_bot_relay_endpoint",
		columns:  youbanBotRelayEndpointColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotRelayEndpointDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotRelayEndpointDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotRelayEndpointDao) Columns() YoubanBotRelayEndpointColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotRelayEndpointDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotRelayEndpointDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotRelayEndpointDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
