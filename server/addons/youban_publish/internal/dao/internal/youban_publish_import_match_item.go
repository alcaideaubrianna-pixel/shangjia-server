// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishImportMatchItemDao is the data access object for the table hg_youban_publish_import_match_item.
type YoubanPublishImportMatchItemDao struct {
	table    string                              // table is the underlying table name of the DAO.
	group    string                              // group is the database configuration group name of the current DAO.
	columns  YoubanPublishImportMatchItemColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                  // handlers for customized model modification.
}

// YoubanPublishImportMatchItemColumns defines and stores column names for the table hg_youban_publish_import_match_item.
type YoubanPublishImportMatchItemColumns struct {
	Id              string //
	MatchRunId      string //
	ImportRunId     string //
	TenantId        string //
	AccountId       string //
	ProfileId       string //
	TaskId          string //
	ChannelId       string //
	DisplayGroupKey string //
	VerifyGroupKey  string //
	DisplayScore    string //
	VerifyScore     string //
	TotalScore      string //
	MatchStatus     string //
	MatchMode       string //
	ReasonJson      string //
	CreatedAt       string //
	UpdatedAt       string //
	DeletedAt       string //
}

// youbanPublishImportMatchItemColumns holds the columns for the table hg_youban_publish_import_match_item.
var youbanPublishImportMatchItemColumns = YoubanPublishImportMatchItemColumns{
	Id:              "id",
	MatchRunId:      "match_run_id",
	ImportRunId:     "import_run_id",
	TenantId:        "tenant_id",
	AccountId:       "account_id",
	ProfileId:       "profile_id",
	TaskId:          "task_id",
	ChannelId:       "channel_id",
	DisplayGroupKey: "display_group_key",
	VerifyGroupKey:  "verify_group_key",
	DisplayScore:    "display_score",
	VerifyScore:     "verify_score",
	TotalScore:      "total_score",
	MatchStatus:     "match_status",
	MatchMode:       "match_mode",
	ReasonJson:      "reason_json",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
	DeletedAt:       "deleted_at",
}

// NewYoubanPublishImportMatchItemDao creates and returns a new DAO object for table data access.
func NewYoubanPublishImportMatchItemDao(handlers ...gdb.ModelHandler) *YoubanPublishImportMatchItemDao {
	return &YoubanPublishImportMatchItemDao{
		group:    "default",
		table:    "hg_youban_publish_import_match_item",
		columns:  youbanPublishImportMatchItemColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishImportMatchItemDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishImportMatchItemDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishImportMatchItemDao) Columns() YoubanPublishImportMatchItemColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishImportMatchItemDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishImportMatchItemDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishImportMatchItemDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
