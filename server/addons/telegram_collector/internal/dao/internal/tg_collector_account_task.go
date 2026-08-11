// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// TgCollectorAccountTaskDao is the data access object for the table hg_tg_collector_account_task.
type TgCollectorAccountTaskDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  TgCollectorAccountTaskColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// TgCollectorAccountTaskColumns defines and stores column names for the table hg_tg_collector_account_task.
type TgCollectorAccountTaskColumns struct {
	Id                  string //
	TenantId            string //
	AccountId           string //
	TaskType            string //
	TaskKey             string //
	Priority            string //
	Status              string //
	AttemptCount        string //
	MaxAttempts         string //
	NextRunAt           string //
	LeaseOwner          string //
	LeaseEpoch          string //
	LeaseUntil          string //
	ErrorMessage        string //
	CompletedAt         string //
	CreatedAt           string //
	UpdatedAt           string //
	HistoryTaskId       string //
	MediaOwnerAccountId string //
	MediaType           string //
	MediaPurpose        string //
	SourceFileId        string //
	FileUrl             string //
	StoragePath         string //
	PosterUrl           string //
	FileMd5             string //
	FilePhash           string //
	SourceKind          string //
	SourceMediaId       string //
	SourceAccessHash    string //
	SourceFileReference string //
	SourceThumbSize     string //
	SourceMimeType      string //
	SourceDcId          string //
	SourceSize          string //
	DebugMetaText       string //
	AttachmentId        string //
	ResultErrorCode     string //
}

// tgCollectorAccountTaskColumns holds the columns for the table hg_tg_collector_account_task.
var tgCollectorAccountTaskColumns = TgCollectorAccountTaskColumns{
	Id:                  "id",
	TenantId:            "tenant_id",
	AccountId:           "account_id",
	TaskType:            "task_type",
	TaskKey:             "task_key",
	Priority:            "priority",
	Status:              "status",
	AttemptCount:        "attempt_count",
	MaxAttempts:         "max_attempts",
	NextRunAt:           "next_run_at",
	LeaseOwner:          "lease_owner",
	LeaseEpoch:          "lease_epoch",
	LeaseUntil:          "lease_until",
	ErrorMessage:        "error_message",
	CompletedAt:         "completed_at",
	CreatedAt:           "created_at",
	UpdatedAt:           "updated_at",
	HistoryTaskId:       "history_task_id",
	MediaOwnerAccountId: "media_owner_account_id",
	MediaType:           "media_type",
	MediaPurpose:        "media_purpose",
	SourceFileId:        "source_file_id",
	FileUrl:             "file_url",
	StoragePath:         "storage_path",
	PosterUrl:           "poster_url",
	FileMd5:             "file_md5",
	FilePhash:           "file_phash",
	SourceKind:          "source_kind",
	SourceMediaId:       "source_media_id",
	SourceAccessHash:    "source_access_hash",
	SourceFileReference: "source_file_reference",
	SourceThumbSize:     "source_thumb_size",
	SourceMimeType:      "source_mime_type",
	SourceDcId:          "source_dc_id",
	SourceSize:          "source_size",
	DebugMetaText:       "debug_meta_text",
	AttachmentId:        "attachment_id",
	ResultErrorCode:     "result_error_code",
}

// NewTgCollectorAccountTaskDao creates and returns a new DAO object for table data access.
func NewTgCollectorAccountTaskDao(handlers ...gdb.ModelHandler) *TgCollectorAccountTaskDao {
	return &TgCollectorAccountTaskDao{
		group:    "default",
		table:    "hg_tg_collector_account_task",
		columns:  tgCollectorAccountTaskColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *TgCollectorAccountTaskDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *TgCollectorAccountTaskDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *TgCollectorAccountTaskDao) Columns() TgCollectorAccountTaskColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *TgCollectorAccountTaskDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *TgCollectorAccountTaskDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *TgCollectorAccountTaskDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
