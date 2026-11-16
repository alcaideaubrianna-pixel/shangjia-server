// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgAttemptDao is the data access object for the table hg_youban_publish_tg_attempt.
type YoubanPublishTgAttemptDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgAttemptColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanPublishTgAttemptColumns defines and stores column names for the table hg_youban_publish_tg_attempt.
type YoubanPublishTgAttemptColumns struct {
	Id               string //
	JobId            string //
	TenantId         string //
	BotId            string //
	ChannelId        string //
	TargetChatId     string //
	Phase            string //
	ChunkIndex       string //
	AttemptNo        string //
	AttemptToken     string //
	ExpectedCount    string //
	ConfirmedCount   string //
	Status           string //
	ResponseReceived string //
	WebhookDeadline  string //
	ErrorMessage     string //
	CreatedAt        string //
	UpdatedAt        string //
}

// youbanPublishTgAttemptColumns holds the columns for the table hg_youban_publish_tg_attempt.
var youbanPublishTgAttemptColumns = YoubanPublishTgAttemptColumns{
	Id:               "id",
	JobId:            "job_id",
	TenantId:         "tenant_id",
	BotId:            "bot_id",
	ChannelId:        "channel_id",
	TargetChatId:     "target_chat_id",
	Phase:            "phase",
	ChunkIndex:       "chunk_index",
	AttemptNo:        "attempt_no",
	AttemptToken:     "attempt_token",
	ExpectedCount:    "expected_count",
	ConfirmedCount:   "confirmed_count",
	Status:           "status",
	ResponseReceived: "response_received",
	WebhookDeadline:  "webhook_deadline",
	ErrorMessage:     "error_message",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
}

// NewYoubanPublishTgAttemptDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgAttemptDao(handlers ...gdb.ModelHandler) *YoubanPublishTgAttemptDao {
	return &YoubanPublishTgAttemptDao{
		group:    "default",
		table:    "hg_youban_publish_tg_attempt",
		columns:  youbanPublishTgAttemptColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgAttemptDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgAttemptDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgAttemptDao) Columns() YoubanPublishTgAttemptColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgAttemptDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgAttemptDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgAttemptDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
