// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotManagedChannelDao is the data access object for the table hg_youban_bot_managed_channel.
type YoubanBotManagedChannelDao struct {
	table    string                         // table is the underlying table name of the DAO.
	group    string                         // group is the database configuration group name of the current DAO.
	columns  YoubanBotManagedChannelColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler             // handlers for customized model modification.
}

// YoubanBotManagedChannelColumns defines and stores column names for the table hg_youban_bot_managed_channel.
type YoubanBotManagedChannelColumns struct {
	Id               string //
	TenantId         string //
	AccountId        string //
	PublishChannelId string //
	TgAccountId      string //
	ChatId           string //
	Title            string //
	AccessHash       string //
	VisitUrl         string //
	AdminCount       string //
	AdminLimit       string //
	BotCount         string //
	CanManage        string //
	SyncStatus       string //
	ErrorMessage     string //
	LastSyncedAt     string //
	LastAssignedAt   string //
	CreatedAt        string //
	UpdatedAt        string //
}

// youbanBotManagedChannelColumns holds the columns for the table hg_youban_bot_managed_channel.
var youbanBotManagedChannelColumns = YoubanBotManagedChannelColumns{
	Id:               "id",
	TenantId:         "tenant_id",
	AccountId:        "account_id",
	PublishChannelId: "publish_channel_id",
	TgAccountId:      "tg_account_id",
	ChatId:           "chat_id",
	Title:            "title",
	AccessHash:       "access_hash",
	VisitUrl:         "visit_url",
	AdminCount:       "admin_count",
	AdminLimit:       "admin_limit",
	BotCount:         "bot_count",
	CanManage:        "can_manage",
	SyncStatus:       "sync_status",
	ErrorMessage:     "error_message",
	LastSyncedAt:     "last_synced_at",
	LastAssignedAt:   "last_assigned_at",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
}

// NewYoubanBotManagedChannelDao creates and returns a new DAO object for table data access.
func NewYoubanBotManagedChannelDao(handlers ...gdb.ModelHandler) *YoubanBotManagedChannelDao {
	return &YoubanBotManagedChannelDao{
		group:    "default",
		table:    "hg_youban_bot_managed_channel",
		columns:  youbanBotManagedChannelColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotManagedChannelDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotManagedChannelDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotManagedChannelDao) Columns() YoubanBotManagedChannelColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotManagedChannelDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotManagedChannelDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotManagedChannelDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
