// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishRelayFileDao is the data access object for the table hg_youban_publish_relay_file.
type YoubanPublishRelayFileDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanPublishRelayFileColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanPublishRelayFileColumns defines and stores column names for the table hg_youban_publish_relay_file.
type YoubanPublishRelayFileColumns struct {
	Id             string //
	TenantId       string //
	TelegramUserId string //
	AssetKey       string //
	FileId         string //
	FileUniqueId   string //
	CreatedAt      string //
	UpdatedAt      string //
}

// youbanPublishRelayFileColumns holds the columns for the table hg_youban_publish_relay_file.
var youbanPublishRelayFileColumns = YoubanPublishRelayFileColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	TelegramUserId: "telegram_user_id",
	AssetKey:       "asset_key",
	FileId:         "file_id",
	FileUniqueId:   "file_unique_id",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
}

// NewYoubanPublishRelayFileDao creates and returns a new DAO object for table data access.
func NewYoubanPublishRelayFileDao(handlers ...gdb.ModelHandler) *YoubanPublishRelayFileDao {
	return &YoubanPublishRelayFileDao{
		group:    "default",
		table:    "hg_youban_publish_relay_file",
		columns:  youbanPublishRelayFileColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishRelayFileDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishRelayFileDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishRelayFileDao) Columns() YoubanPublishRelayFileColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishRelayFileDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishRelayFileDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishRelayFileDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
