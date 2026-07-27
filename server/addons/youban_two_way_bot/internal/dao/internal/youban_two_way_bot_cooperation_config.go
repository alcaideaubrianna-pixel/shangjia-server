// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanTwoWayBotCooperationConfigDao is the data access object for the table hg_youban_two_way_bot_cooperation_config.
type YoubanTwoWayBotCooperationConfigDao struct {
	table    string                                  // table is the underlying table name of the DAO.
	group    string                                  // group is the database configuration group name of the current DAO.
	columns  YoubanTwoWayBotCooperationConfigColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                      // handlers for customized model modification.
}

// YoubanTwoWayBotCooperationConfigColumns defines and stores column names for the table hg_youban_two_way_bot_cooperation_config.
type YoubanTwoWayBotCooperationConfigColumns struct {
	Id               string //
	TenantId         string //
	AccountId        string //
	BotId            string //
	TwoWayBotId      string //
	NotificationType string //
	ReviewRequired   string //
	Status           string //
	CreatedBy        string //
	UpdatedBy        string //
	CreatedAt        string //
	UpdatedAt        string //
	DeletedAt        string //
}

// youbanTwoWayBotCooperationConfigColumns holds the columns for the table hg_youban_two_way_bot_cooperation_config.
var youbanTwoWayBotCooperationConfigColumns = YoubanTwoWayBotCooperationConfigColumns{
	Id:               "id",
	TenantId:         "tenant_id",
	AccountId:        "account_id",
	BotId:            "bot_id",
	TwoWayBotId:      "two_way_bot_id",
	NotificationType: "notification_type",
	ReviewRequired:   "review_required",
	Status:           "status",
	CreatedBy:        "created_by",
	UpdatedBy:        "updated_by",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
	DeletedAt:        "deleted_at",
}

// NewYoubanTwoWayBotCooperationConfigDao creates and returns a new DAO object for table data access.
func NewYoubanTwoWayBotCooperationConfigDao(handlers ...gdb.ModelHandler) *YoubanTwoWayBotCooperationConfigDao {
	return &YoubanTwoWayBotCooperationConfigDao{
		group:    "default",
		table:    "hg_youban_two_way_bot_cooperation_config",
		columns:  youbanTwoWayBotCooperationConfigColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanTwoWayBotCooperationConfigDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanTwoWayBotCooperationConfigDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanTwoWayBotCooperationConfigDao) Columns() YoubanTwoWayBotCooperationConfigColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanTwoWayBotCooperationConfigDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanTwoWayBotCooperationConfigDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanTwoWayBotCooperationConfigDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
