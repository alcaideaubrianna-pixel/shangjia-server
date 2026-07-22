// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishMaterialImportTaskDao is the data access object for the table hg_youban_publish_material_import_task.
type YoubanPublishMaterialImportTaskDao struct {
	table    string                                 // table is the underlying table name of the DAO.
	group    string                                 // group is the database configuration group name of the current DAO.
	columns  YoubanPublishMaterialImportTaskColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                     // handlers for customized model modification.
}

// YoubanPublishMaterialImportTaskColumns defines and stores column names for the table hg_youban_publish_material_import_task.
type YoubanPublishMaterialImportTaskColumns struct {
	Id             string //
	TenantId       string //
	AccountId      string //
	TgAccountId    string //
	SourceChatId   string //
	SourceTitle    string //
	SourceUsername string //
	Status         string //
	Stage          string //
	PullOffsetId   string //
	PullLimitDays  string //
	MessageTotal   string //
	MessageDone    string //
	GroupTotal     string //
	GroupDone      string //
	MediaTotal     string //
	MediaDone      string //
	MediaFailed    string //
	Imported       string //
	Duplicate      string //
	ErrorMessage   string //
	NextRunAt      string //
	ResultJson     string //
	CreatedBy      string //
	UpdatedBy      string //
	StartedAt      string //
	FinishedAt     string //
	CreatedAt      string //
	UpdatedAt      string //
}

// youbanPublishMaterialImportTaskColumns holds the columns for the table hg_youban_publish_material_import_task.
var youbanPublishMaterialImportTaskColumns = YoubanPublishMaterialImportTaskColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	AccountId:      "account_id",
	TgAccountId:    "tg_account_id",
	SourceChatId:   "source_chat_id",
	SourceTitle:    "source_title",
	SourceUsername: "source_username",
	Status:         "status",
	Stage:          "stage",
	PullOffsetId:   "pull_offset_id",
	PullLimitDays:  "pull_limit_days",
	MessageTotal:   "message_total",
	MessageDone:    "message_done",
	GroupTotal:     "group_total",
	GroupDone:      "group_done",
	MediaTotal:     "media_total",
	MediaDone:      "media_done",
	MediaFailed:    "media_failed",
	Imported:       "imported",
	Duplicate:      "duplicate",
	ErrorMessage:   "error_message",
	NextRunAt:      "next_run_at",
	ResultJson:     "result_json",
	CreatedBy:      "created_by",
	UpdatedBy:      "updated_by",
	StartedAt:      "started_at",
	FinishedAt:     "finished_at",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
}

// NewYoubanPublishMaterialImportTaskDao creates and returns a new DAO object for table data access.
func NewYoubanPublishMaterialImportTaskDao(handlers ...gdb.ModelHandler) *YoubanPublishMaterialImportTaskDao {
	return &YoubanPublishMaterialImportTaskDao{
		group:    "default",
		table:    "hg_youban_publish_material_import_task",
		columns:  youbanPublishMaterialImportTaskColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishMaterialImportTaskDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishMaterialImportTaskDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishMaterialImportTaskDao) Columns() YoubanPublishMaterialImportTaskColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishMaterialImportTaskDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishMaterialImportTaskDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishMaterialImportTaskDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
