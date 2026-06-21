// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// ContentImportRunDao is the data access object for the table hg_content_import_run.
type ContentImportRunDao struct {
	table    string                  // table is the underlying table name of the DAO.
	group    string                  // group is the database configuration group name of the current DAO.
	columns  ContentImportRunColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler      // handlers for customized model modification.
}

// ContentImportRunColumns defines and stores column names for the table hg_content_import_run.
type ContentImportRunColumns struct {
	Id               string // ID
	SourceName       string // 来源名称
	TriggerType      string // 触发方式
	BatchSize        string // 批量数量
	Scanned          string // 扫描数量
	Imported         string // 导入数量
	Duplicate        string // 重复数量
	MediaImported    string // 媒体导入数量
	LastSourceNoteId string // 最后来源笔记ID
	Status           string // 运行状态
	ErrorMessage     string // 错误信息
	StartedAt        string // 开始时间
	FinishedAt       string // 结束时间
	CostMs           string // 耗时毫秒
	CreatedAt        string // 创建时间
	UpdatedAt        string // 更新时间
}

// contentImportRunColumns holds the columns for the table hg_content_import_run.
var contentImportRunColumns = ContentImportRunColumns{
	Id:               "id",
	SourceName:       "source_name",
	TriggerType:      "trigger_type",
	BatchSize:        "batch_size",
	Scanned:          "scanned",
	Imported:         "imported",
	Duplicate:        "duplicate",
	MediaImported:    "media_imported",
	LastSourceNoteId: "last_source_note_id",
	Status:           "status",
	ErrorMessage:     "error_message",
	StartedAt:        "started_at",
	FinishedAt:       "finished_at",
	CostMs:           "cost_ms",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
}

// NewContentImportRunDao creates and returns a new DAO object for table data access.
func NewContentImportRunDao(handlers ...gdb.ModelHandler) *ContentImportRunDao {
	return &ContentImportRunDao{
		group:    "default",
		table:    "hg_content_import_run",
		columns:  contentImportRunColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *ContentImportRunDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *ContentImportRunDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *ContentImportRunDao) Columns() ContentImportRunColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *ContentImportRunDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *ContentImportRunDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *ContentImportRunDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
