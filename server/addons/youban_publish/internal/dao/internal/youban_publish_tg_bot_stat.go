// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgBotStatDao is the data access object for the table hg_youban_publish_tg_bot_stat.
type YoubanPublishTgBotStatDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgBotStatColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanPublishTgBotStatColumns defines and stores column names for the table hg_youban_publish_tg_bot_stat.
type YoubanPublishTgBotStatColumns struct {
	Id               string //
	TenantId         string //
	BotId            string //
	BotName          string //
	BotUsername      string //
	PendingCount     string //
	QueuedCount      string //
	SendingCount     string //
	SentCount        string //
	FailedCount      string //
	RetryCount       string //
	RateLimitCount   string //
	LastSentAt       string //
	LastErrorAt      string //
	LastErrorMessage string //
	CreatedAt        string //
	UpdatedAt        string //
}

// youbanPublishTgBotStatColumns holds the columns for the table hg_youban_publish_tg_bot_stat.
var youbanPublishTgBotStatColumns = YoubanPublishTgBotStatColumns{
	Id:               "id",
	TenantId:         "tenant_id",
	BotId:            "bot_id",
	BotName:          "bot_name",
	BotUsername:      "bot_username",
	PendingCount:     "pending_count",
	QueuedCount:      "queued_count",
	SendingCount:     "sending_count",
	SentCount:        "sent_count",
	FailedCount:      "failed_count",
	RetryCount:       "retry_count",
	RateLimitCount:   "rate_limit_count",
	LastSentAt:       "last_sent_at",
	LastErrorAt:      "last_error_at",
	LastErrorMessage: "last_error_message",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
}

// NewYoubanPublishTgBotStatDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgBotStatDao(handlers ...gdb.ModelHandler) *YoubanPublishTgBotStatDao {
	return &YoubanPublishTgBotStatDao{
		group:    "default",
		table:    "hg_youban_publish_tg_bot_stat",
		columns:  youbanPublishTgBotStatColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgBotStatDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgBotStatDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgBotStatDao) Columns() YoubanPublishTgBotStatColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgBotStatDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgBotStatDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgBotStatDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
