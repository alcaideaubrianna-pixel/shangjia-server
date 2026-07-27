// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanTwoWayBotCooperationChannelDao is the data access object for the table hg_youban_two_way_bot_cooperation_channel.
type YoubanTwoWayBotCooperationChannelDao struct {
	table    string                                   // table is the underlying table name of the DAO.
	group    string                                   // group is the database configuration group name of the current DAO.
	columns  YoubanTwoWayBotCooperationChannelColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                       // handlers for customized model modification.
}

// YoubanTwoWayBotCooperationChannelColumns defines and stores column names for the table hg_youban_two_way_bot_cooperation_channel.
type YoubanTwoWayBotCooperationChannelColumns struct {
	Id        string //
	TenantId  string //
	ConfigId  string //
	ChannelId string //
	Status    string //
	CreatedAt string //
	UpdatedAt string //
	DeletedAt string //
}

// youbanTwoWayBotCooperationChannelColumns holds the columns for the table hg_youban_two_way_bot_cooperation_channel.
var youbanTwoWayBotCooperationChannelColumns = YoubanTwoWayBotCooperationChannelColumns{
	Id:        "id",
	TenantId:  "tenant_id",
	ConfigId:  "config_id",
	ChannelId: "channel_id",
	Status:    "status",
	CreatedAt: "created_at",
	UpdatedAt: "updated_at",
	DeletedAt: "deleted_at",
}

// NewYoubanTwoWayBotCooperationChannelDao creates and returns a new DAO object for table data access.
func NewYoubanTwoWayBotCooperationChannelDao(handlers ...gdb.ModelHandler) *YoubanTwoWayBotCooperationChannelDao {
	return &YoubanTwoWayBotCooperationChannelDao{
		group:    "default",
		table:    "hg_youban_two_way_bot_cooperation_channel",
		columns:  youbanTwoWayBotCooperationChannelColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanTwoWayBotCooperationChannelDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanTwoWayBotCooperationChannelDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanTwoWayBotCooperationChannelDao) Columns() YoubanTwoWayBotCooperationChannelColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanTwoWayBotCooperationChannelDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanTwoWayBotCooperationChannelDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanTwoWayBotCooperationChannelDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
