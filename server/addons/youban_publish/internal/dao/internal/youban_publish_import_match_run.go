// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishImportMatchRunDao is the data access object for the table hg_youban_publish_import_match_run.
type YoubanPublishImportMatchRunDao struct {
	table    string                             // table is the underlying table name of the DAO.
	group    string                             // group is the database configuration group name of the current DAO.
	columns  YoubanPublishImportMatchRunColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                 // handlers for customized model modification.
}

// YoubanPublishImportMatchRunColumns defines and stores column names for the table hg_youban_publish_import_match_run.
type YoubanPublishImportMatchRunColumns struct {
	Id             string //
	ImportRunId    string //
	TenantId       string //
	AccountId      string //
	Status         string //
	Stage          string //
	ChannelIdJson  string //
	ScanDays       string //
	Threshold      string //
	ProfileTotal   string //
	ProfileDone    string //
	CandidateTotal string //
	AutoMatched    string //
	ManualPending  string //
	Confirmed      string //
	Skipped        string //
	ErrorMessage   string //
	CreatedAt      string //
	UpdatedAt      string //
	FinishedAt     string //
	DeletedAt      string //
}

// youbanPublishImportMatchRunColumns holds the columns for the table hg_youban_publish_import_match_run.
var youbanPublishImportMatchRunColumns = YoubanPublishImportMatchRunColumns{
	Id:             "id",
	ImportRunId:    "import_run_id",
	TenantId:       "tenant_id",
	AccountId:      "account_id",
	Status:         "status",
	Stage:          "stage",
	ChannelIdJson:  "channel_id_json",
	ScanDays:       "scan_days",
	Threshold:      "threshold",
	ProfileTotal:   "profile_total",
	ProfileDone:    "profile_done",
	CandidateTotal: "candidate_total",
	AutoMatched:    "auto_matched",
	ManualPending:  "manual_pending",
	Confirmed:      "confirmed",
	Skipped:        "skipped",
	ErrorMessage:   "error_message",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
	FinishedAt:     "finished_at",
	DeletedAt:      "deleted_at",
}

// NewYoubanPublishImportMatchRunDao creates and returns a new DAO object for table data access.
func NewYoubanPublishImportMatchRunDao(handlers ...gdb.ModelHandler) *YoubanPublishImportMatchRunDao {
	return &YoubanPublishImportMatchRunDao{
		group:    "default",
		table:    "hg_youban_publish_import_match_run",
		columns:  youbanPublishImportMatchRunColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishImportMatchRunDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishImportMatchRunDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishImportMatchRunDao) Columns() YoubanPublishImportMatchRunColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishImportMatchRunDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishImportMatchRunDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishImportMatchRunDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
