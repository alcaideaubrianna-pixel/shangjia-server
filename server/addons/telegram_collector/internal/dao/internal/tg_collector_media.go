// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// TgCollectorMediaDao is the data access object for the table hg_tg_collector_media.
type TgCollectorMediaDao struct {
	table    string                  // table is the underlying table name of the DAO.
	group    string                  // group is the database configuration group name of the current DAO.
	columns  TgCollectorMediaColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler      // handlers for customized model modification.
}

// TgCollectorMediaColumns defines and stores column names for the table hg_tg_collector_media.
type TgCollectorMediaColumns struct {
	Id                string //
	TenantId          string //
	Fingerprint       string //
	Kind              string //
	MimeType          string //
	Size              string //
	PipelineVersion   string //
	Status            string //
	FileURL           string //
	StoragePath       string //
	PosterURL         string //
	PosterStoragePath string //
	Phash             string //
	Dhash             string //
	AttemptCount      string //
	NextRunAt         string //
	LeaseOwner        string //
	LeaseUntil        string //
	ErrorMessage      string //
	CreatedAt         string //
	UpdatedAt         string //
}

// tgCollectorMediaColumns holds the columns for the table hg_tg_collector_media.
var tgCollectorMediaColumns = TgCollectorMediaColumns{
	Id:                "id",
	TenantId:          "tenant_id",
	Fingerprint:       "fingerprint",
	Kind:              "kind",
	MimeType:          "mime_type",
	Size:              "size",
	PipelineVersion:   "pipeline_version",
	Status:            "status",
	FileURL:           "file_url",
	StoragePath:       "storage_path",
	PosterURL:         "poster_url",
	PosterStoragePath: "poster_storage_path",
	Phash:             "phash",
	Dhash:             "dhash",
	AttemptCount:      "attempt_count",
	NextRunAt:         "next_run_at",
	LeaseOwner:        "lease_owner",
	LeaseUntil:        "lease_until",
	ErrorMessage:      "error_message",
	CreatedAt:         "created_at",
	UpdatedAt:         "updated_at",
}

// NewTgCollectorMediaDao creates and returns a new DAO object for table data access.
func NewTgCollectorMediaDao(handlers ...gdb.ModelHandler) *TgCollectorMediaDao {
	return &TgCollectorMediaDao{
		group:    "default",
		table:    "hg_tg_collector_media",
		columns:  tgCollectorMediaColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *TgCollectorMediaDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *TgCollectorMediaDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *TgCollectorMediaDao) Columns() TgCollectorMediaColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *TgCollectorMediaDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *TgCollectorMediaDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *TgCollectorMediaDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
