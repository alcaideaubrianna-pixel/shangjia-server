// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotUserDao is the data access object for the table hg_youban_bot_user.
type YoubanBotUserDao struct {
	table    string               // table is the underlying table name of the DAO.
	group    string               // group is the database configuration group name of the current DAO.
	columns  YoubanBotUserColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler   // handlers for customized model modification.
}

// YoubanBotUserColumns defines and stores column names for the table hg_youban_bot_user.
type YoubanBotUserColumns struct {
	Id                string //
	BotId             string //
	TelegramUserId    string //
	TelegramUsername  string //
	TelegramFirstName string //
	TelegramLastName  string //
	ChatId            string //
	ChatType          string //
	ChatTitle         string //
	MessageCount      string //
	LastMessageText   string //
	LastMessageAt     string //
	Status            string //
	IsSuperAdmin      string //
	CreatedAt         string //
	UpdatedAt         string //
}

// youbanBotUserColumns holds the columns for the table hg_youban_bot_user.
var youbanBotUserColumns = YoubanBotUserColumns{
	Id:                "id",
	BotId:             "bot_id",
	TelegramUserId:    "telegram_user_id",
	TelegramUsername:  "telegram_username",
	TelegramFirstName: "telegram_first_name",
	TelegramLastName:  "telegram_last_name",
	ChatId:            "chat_id",
	ChatType:          "chat_type",
	ChatTitle:         "chat_title",
	MessageCount:      "message_count",
	LastMessageText:   "last_message_text",
	LastMessageAt:     "last_message_at",
	Status:            "status",
	IsSuperAdmin:      "is_super_admin",
	CreatedAt:         "created_at",
	UpdatedAt:         "updated_at",
}

// NewYoubanBotUserDao creates and returns a new DAO object for table data access.
func NewYoubanBotUserDao(handlers ...gdb.ModelHandler) *YoubanBotUserDao {
	return &YoubanBotUserDao{
		group:    "default",
		table:    "hg_youban_bot_user",
		columns:  youbanBotUserColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotUserDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotUserDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotUserDao) Columns() YoubanBotUserColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotUserDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotUserDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotUserDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
