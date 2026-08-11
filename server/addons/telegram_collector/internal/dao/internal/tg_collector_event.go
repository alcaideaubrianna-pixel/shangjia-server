// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// TgCollectorEventDao is the data access object for the table hg_tg_collector_event.
type TgCollectorEventDao struct {
	table    string                  // table is the underlying table name of the DAO.
	group    string                  // group is the database configuration group name of the current DAO.
	columns  TgCollectorEventColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler      // handlers for customized model modification.
}

// TgCollectorEventColumns defines and stores column names for the table hg_tg_collector_event.
type TgCollectorEventColumns struct {
	Id           string //
	TenantId     string //
	SourceId     string //
	SourceType   string //
	BotKey       string //
	AccountId    string //
	ChatId       string //
	MessageId    string //
	UpdateId     string //
	EventKey     string //
	RawUpdate    string //
	Priority     string //
	Status       string //
	AttemptCount string //
	NextRunAt    string //
	LeaseOwner   string //
	LeaseUntil   string //
	ReceivedAt   string //
	ProcessedAt  string //
	ErrorMessage string //
	CreatedAt    string //
	UpdatedAt    string //
}

// tgCollectorEventColumns holds the columns for the table hg_tg_collector_event.
var tgCollectorEventColumns = TgCollectorEventColumns{
	Id:           "id",
	TenantId:     "tenant_id",
	SourceId:     "source_id",
	SourceType:   "source_type",
	BotKey:       "bot_key",
	AccountId:    "account_id",
	ChatId:       "chat_id",
	MessageId:    "message_id",
	UpdateId:     "update_id",
	EventKey:     "event_key",
	RawUpdate:    "raw_update",
	Priority:     "priority",
	Status:       "status",
	AttemptCount: "attempt_count",
	NextRunAt:    "next_run_at",
	LeaseOwner:   "lease_owner",
	LeaseUntil:   "lease_until",
	ReceivedAt:   "received_at",
	ProcessedAt:  "processed_at",
	ErrorMessage: "error_message",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
}

// NewTgCollectorEventDao creates and returns a new DAO object for table data access.
func NewTgCollectorEventDao(handlers ...gdb.ModelHandler) *TgCollectorEventDao {
	return &TgCollectorEventDao{
		group:    "default",
		table:    "hg_tg_collector_event",
		columns:  tgCollectorEventColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *TgCollectorEventDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *TgCollectorEventDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *TgCollectorEventDao) Columns() TgCollectorEventColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *TgCollectorEventDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *TgCollectorEventDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *TgCollectorEventDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
