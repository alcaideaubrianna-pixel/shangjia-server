// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotChannelCacheDao is the data access object for the table hg_youban_bot_channel_cache.
type YoubanBotChannelCacheDao struct {
	table    string                       // table is the underlying table name of the DAO.
	group    string                       // group is the database configuration group name of the current DAO.
	columns  YoubanBotChannelCacheColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler           // handlers for customized model modification.
}

// YoubanBotChannelCacheColumns defines and stores column names for the table hg_youban_bot_channel_cache.
type YoubanBotChannelCacheColumns struct {
	Id              string //
	BotId           string //
	ChatId          string //
	ChatType        string //
	ChatTitle       string //
	ChatUsername    string //
	IsBroadcast     string //
	IsMegagroup     string //
	MessageCount    string //
	LastMessageText string //
	LastMessageAt   string //
	CreatedAt       string //
	UpdatedAt       string //
}

// youbanBotChannelCacheColumns holds the columns for the table hg_youban_bot_channel_cache.
var youbanBotChannelCacheColumns = YoubanBotChannelCacheColumns{
	Id:              "id",
	BotId:           "bot_id",
	ChatId:          "chat_id",
	ChatType:        "chat_type",
	ChatTitle:       "chat_title",
	ChatUsername:    "chat_username",
	IsBroadcast:     "is_broadcast",
	IsMegagroup:     "is_megagroup",
	MessageCount:    "message_count",
	LastMessageText: "last_message_text",
	LastMessageAt:   "last_message_at",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
}

// NewYoubanBotChannelCacheDao creates and returns a new DAO object for table data access.
func NewYoubanBotChannelCacheDao(handlers ...gdb.ModelHandler) *YoubanBotChannelCacheDao {
	return &YoubanBotChannelCacheDao{
		group:    "default",
		table:    "hg_youban_bot_channel_cache",
		columns:  youbanBotChannelCacheColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotChannelCacheDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotChannelCacheDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotChannelCacheDao) Columns() YoubanBotChannelCacheColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotChannelCacheDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotChannelCacheDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotChannelCacheDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
