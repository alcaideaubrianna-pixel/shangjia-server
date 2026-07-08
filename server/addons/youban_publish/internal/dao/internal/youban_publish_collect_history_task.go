// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectHistoryTaskDao is the data access object for the table hg_youban_publish_collect_history_task.
type YoubanPublishCollectHistoryTaskDao struct {
	table    string                                 // table is the underlying table name of the DAO.
	group    string                                 // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectHistoryTaskColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                     // handlers for customized model modification.
}

// YoubanPublishCollectHistoryTaskColumns defines and stores column names for the table hg_youban_publish_collect_history_task.
type YoubanPublishCollectHistoryTaskColumns struct {
	Id             string //
	TenantId       string //
	AccountId      string //
	SourceId       string //
	TgAccountId    string //
	SourceChatId   string //
	Mode           string //
	Days           string //
	OffsetId       string //
	ScannedCount   string //
	EventCount     string //
	DuplicateCount string //
	FailedCount    string //
	Status         string //
	ErrorMessage   string //
	NextRunAt      string //
	StartedAt      string //
	FinishedAt     string //
	CreatedAt      string //
	UpdatedAt      string //
}

// youbanPublishCollectHistoryTaskColumns holds the columns for the table hg_youban_publish_collect_history_task.
var youbanPublishCollectHistoryTaskColumns = YoubanPublishCollectHistoryTaskColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	AccountId:      "account_id",
	SourceId:       "source_id",
	TgAccountId:    "tg_account_id",
	SourceChatId:   "source_chat_id",
	Mode:           "mode",
	Days:           "days",
	OffsetId:       "offset_id",
	ScannedCount:   "scanned_count",
	EventCount:     "event_count",
	DuplicateCount: "duplicate_count",
	FailedCount:    "failed_count",
	Status:         "status",
	ErrorMessage:   "error_message",
	NextRunAt:      "next_run_at",
	StartedAt:      "started_at",
	FinishedAt:     "finished_at",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
}

// NewYoubanPublishCollectHistoryTaskDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectHistoryTaskDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectHistoryTaskDao {
	return &YoubanPublishCollectHistoryTaskDao{
		group:    "default",
		table:    "hg_youban_publish_collect_history_task",
		columns:  youbanPublishCollectHistoryTaskColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectHistoryTaskDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectHistoryTaskDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectHistoryTaskDao) Columns() YoubanPublishCollectHistoryTaskColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectHistoryTaskDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectHistoryTaskDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectHistoryTaskDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
