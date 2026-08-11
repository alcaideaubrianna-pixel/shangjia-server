// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// TgCollectorSourceDao is the data access object for the table hg_tg_collector_source.
type TgCollectorSourceDao struct {
	table    string                   // table is the underlying table name of the DAO.
	group    string                   // group is the database configuration group name of the current DAO.
	columns  TgCollectorSourceColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler       // handlers for customized model modification.
}

// TgCollectorSourceColumns defines and stores column names for the table hg_tg_collector_source.
type TgCollectorSourceColumns struct {
	Id             string //
	TenantId       string //
	AccountId      string //
	BotId          string //
	SourceType     string //
	ChatId         string //
	ChatTitle      string //
	ChatUsername   string //
	Status         string //
	HistoryEnabled string //
	HistoryCursor  string //
	CreatedAt      string //
	UpdatedAt      string //
}

// tgCollectorSourceColumns holds the columns for the table hg_tg_collector_source.
var tgCollectorSourceColumns = TgCollectorSourceColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	AccountId:      "account_id",
	BotId:          "bot_id",
	SourceType:     "source_type",
	ChatId:         "chat_id",
	ChatTitle:      "chat_title",
	ChatUsername:   "chat_username",
	Status:         "status",
	HistoryEnabled: "history_enabled",
	HistoryCursor:  "history_cursor",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
}

// NewTgCollectorSourceDao creates and returns a new DAO object for table data access.
func NewTgCollectorSourceDao(handlers ...gdb.ModelHandler) *TgCollectorSourceDao {
	return &TgCollectorSourceDao{
		group:    "default",
		table:    "hg_tg_collector_source",
		columns:  tgCollectorSourceColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *TgCollectorSourceDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *TgCollectorSourceDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *TgCollectorSourceDao) Columns() TgCollectorSourceColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *TgCollectorSourceDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *TgCollectorSourceDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *TgCollectorSourceDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
