// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgMessageRepairRunDao is the data access object for the table hg_youban_publish_tg_message_repair_run.
type YoubanPublishTgMessageRepairRunDao struct {
	table    string                                 // table is the underlying table name of the DAO.
	group    string                                 // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgMessageRepairRunColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                     // handlers for customized model modification.
}

// YoubanPublishTgMessageRepairRunColumns defines and stores column names for the table hg_youban_publish_tg_message_repair_run.
type YoubanPublishTgMessageRepairRunColumns struct {
	Id           string //
	TenantId     string //
	AccountId    string //
	ProfileId    string //
	TaskId       string //
	Status       string //
	Stage        string //
	Progress     string //
	ChannelCount string //
	ScannedCount string //
	MatchedCount string //
	ErrorMessage string //
	CreatedAt    string //
	UpdatedAt    string //
	FinishedAt   string //
}

// youbanPublishTgMessageRepairRunColumns holds the columns for the table hg_youban_publish_tg_message_repair_run.
var youbanPublishTgMessageRepairRunColumns = YoubanPublishTgMessageRepairRunColumns{
	Id:           "id",
	TenantId:     "tenant_id",
	AccountId:    "account_id",
	ProfileId:    "profile_id",
	TaskId:       "task_id",
	Status:       "status",
	Stage:        "stage",
	Progress:     "progress",
	ChannelCount: "channel_count",
	ScannedCount: "scanned_count",
	MatchedCount: "matched_count",
	ErrorMessage: "error_message",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
	FinishedAt:   "finished_at",
}

// NewYoubanPublishTgMessageRepairRunDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgMessageRepairRunDao(handlers ...gdb.ModelHandler) *YoubanPublishTgMessageRepairRunDao {
	return &YoubanPublishTgMessageRepairRunDao{
		group:    "default",
		table:    "hg_youban_publish_tg_message_repair_run",
		columns:  youbanPublishTgMessageRepairRunColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgMessageRepairRunDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgMessageRepairRunDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgMessageRepairRunDao) Columns() YoubanPublishTgMessageRepairRunColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgMessageRepairRunDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgMessageRepairRunDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgMessageRepairRunDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
