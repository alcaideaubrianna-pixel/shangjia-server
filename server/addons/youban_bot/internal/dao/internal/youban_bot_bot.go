// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotBotDao is the data access object for the table hg_youban_bot_bot.
type YoubanBotBotDao struct {
	table    string              // table is the underlying table name of the DAO.
	group    string              // group is the database configuration group name of the current DAO.
	columns  YoubanBotBotColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler  // handlers for customized model modification.
}

// YoubanBotBotColumns defines and stores column names for the table hg_youban_bot_bot.
type YoubanBotBotColumns struct {
	Id          string //
	BotName     string //
	BotUsername string //
	BotToken    string //
	IsOfficial  string //
	IsDefault   string //
	RunMode     string //
	WebhookUrl  string //
	Remark      string //
	Status      string //
	CreatedBy   string //
	UpdatedBy   string //
	DeletedBy   string //
	CreatedAt   string //
	UpdatedAt   string //
	DeletedAt   string //
}

// youbanBotBotColumns holds the columns for the table hg_youban_bot_bot.
var youbanBotBotColumns = YoubanBotBotColumns{
	Id:          "id",
	BotName:     "bot_name",
	BotUsername: "bot_username",
	BotToken:    "bot_token",
	IsOfficial:  "is_official",
	IsDefault:   "is_default",
	RunMode:     "run_mode",
	WebhookUrl:  "webhook_url",
	Remark:      "remark",
	Status:      "status",
	CreatedBy:   "created_by",
	UpdatedBy:   "updated_by",
	DeletedBy:   "deleted_by",
	CreatedAt:   "created_at",
	UpdatedAt:   "updated_at",
	DeletedAt:   "deleted_at",
}

// NewYoubanBotBotDao creates and returns a new DAO object for table data access.
func NewYoubanBotBotDao(handlers ...gdb.ModelHandler) *YoubanBotBotDao {
	return &YoubanBotBotDao{
		group:    "default",
		table:    "hg_youban_bot_bot",
		columns:  youbanBotBotColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotBotDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotBotDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotBotDao) Columns() YoubanBotBotColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotBotDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotBotDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotBotDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
