// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishRelayBotDao is the data access object for the table hg_youban_publish_relay_bot.
type YoubanPublishRelayBotDao struct {
	table    string                       // table is the underlying table name of the DAO.
	group    string                       // group is the database configuration group name of the current DAO.
	columns  YoubanPublishRelayBotColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler           // handlers for customized model modification.
}

// YoubanPublishRelayBotColumns defines and stores column names for the table hg_youban_publish_relay_bot.
type YoubanPublishRelayBotColumns struct {
	Id             string //
	TenantId       string //
	RelayChannelId string //
	BotId          string //
	TelegramUserId string //
	Status         string //
	CheckedAt      string //
	LastError      string //
	CreatedAt      string //
	UpdatedAt      string //
}

// youbanPublishRelayBotColumns holds the columns for the table hg_youban_publish_relay_bot.
var youbanPublishRelayBotColumns = YoubanPublishRelayBotColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	RelayChannelId: "relay_channel_id",
	BotId:          "bot_id",
	TelegramUserId: "telegram_user_id",
	Status:         "status",
	CheckedAt:      "checked_at",
	LastError:      "last_error",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
}

// NewYoubanPublishRelayBotDao creates and returns a new DAO object for table data access.
func NewYoubanPublishRelayBotDao(handlers ...gdb.ModelHandler) *YoubanPublishRelayBotDao {
	return &YoubanPublishRelayBotDao{
		group:    "default",
		table:    "hg_youban_publish_relay_bot",
		columns:  youbanPublishRelayBotColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishRelayBotDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishRelayBotDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishRelayBotDao) Columns() YoubanPublishRelayBotColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishRelayBotDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishRelayBotDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishRelayBotDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
