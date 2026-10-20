// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotManagerStateDao is the data access object for the table hg_youban_bot_manager_state.
type YoubanBotManagerStateDao struct {
	table    string                       // table is the underlying table name of the DAO.
	group    string                       // group is the database configuration group name of the current DAO.
	columns  YoubanBotManagerStateColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler           // handlers for customized model modification.
}

// YoubanBotManagerStateColumns defines and stores column names for the table hg_youban_bot_manager_state.
type YoubanBotManagerStateColumns struct {
	Id                 string //
	TenantId           string //
	AccountId          string //
	EntryBotId         string //
	TelegramUserId     string //
	SelectedChannelIds string //
	SelectionSaved     string //
	InputMode          string //
	Keyword            string //
	DraftBotId         string //
	Revision           string //
	SyncedAt           string //
	CreatedAt          string //
	UpdatedAt          string //
	DraftRightsJson    string //
}

// youbanBotManagerStateColumns holds the columns for the table hg_youban_bot_manager_state.
var youbanBotManagerStateColumns = YoubanBotManagerStateColumns{
	Id:                 "id",
	TenantId:           "tenant_id",
	AccountId:          "account_id",
	EntryBotId:         "entry_bot_id",
	TelegramUserId:     "telegram_user_id",
	SelectedChannelIds: "selected_channel_ids",
	SelectionSaved:     "selection_saved",
	InputMode:          "input_mode",
	Keyword:            "keyword",
	DraftBotId:         "draft_bot_id",
	Revision:           "revision",
	SyncedAt:           "synced_at",
	CreatedAt:          "created_at",
	UpdatedAt:          "updated_at",
	DraftRightsJson:    "draft_rights_json",
}

// NewYoubanBotManagerStateDao creates and returns a new DAO object for table data access.
func NewYoubanBotManagerStateDao(handlers ...gdb.ModelHandler) *YoubanBotManagerStateDao {
	return &YoubanBotManagerStateDao{
		group:    "default",
		table:    "hg_youban_bot_manager_state",
		columns:  youbanBotManagerStateColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotManagerStateDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotManagerStateDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotManagerStateDao) Columns() YoubanBotManagerStateColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotManagerStateDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotManagerStateDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotManagerStateDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
