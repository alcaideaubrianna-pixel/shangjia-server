// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgChannelStatDao is the data access object for the table hg_youban_publish_tg_channel_stat.
type YoubanPublishTgChannelStatDao struct {
	table    string                            // table is the underlying table name of the DAO.
	group    string                            // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgChannelStatColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                // handlers for customized model modification.
}

// YoubanPublishTgChannelStatColumns defines and stores column names for the table hg_youban_publish_tg_channel_stat.
type YoubanPublishTgChannelStatColumns struct {
	Id               string //
	TenantId         string //
	AccountId        string //
	ChannelId        string //
	TargetChatId     string //
	ChannelTitle     string //
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

// youbanPublishTgChannelStatColumns holds the columns for the table hg_youban_publish_tg_channel_stat.
var youbanPublishTgChannelStatColumns = YoubanPublishTgChannelStatColumns{
	Id:               "id",
	TenantId:         "tenant_id",
	AccountId:        "account_id",
	ChannelId:        "channel_id",
	TargetChatId:     "target_chat_id",
	ChannelTitle:     "channel_title",
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

// NewYoubanPublishTgChannelStatDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgChannelStatDao(handlers ...gdb.ModelHandler) *YoubanPublishTgChannelStatDao {
	return &YoubanPublishTgChannelStatDao{
		group:    "default",
		table:    "hg_youban_publish_tg_channel_stat",
		columns:  youbanPublishTgChannelStatColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgChannelStatDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgChannelStatDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgChannelStatDao) Columns() YoubanPublishTgChannelStatColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgChannelStatDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgChannelStatDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgChannelStatDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
