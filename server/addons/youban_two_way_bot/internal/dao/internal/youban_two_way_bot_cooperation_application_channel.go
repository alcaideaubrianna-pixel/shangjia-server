// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanTwoWayBotCooperationApplicationChannelDao is the data access object for the table hg_youban_two_way_bot_cooperation_application_channel.
type YoubanTwoWayBotCooperationApplicationChannelDao struct {
	table    string                                              // table is the underlying table name of the DAO.
	group    string                                              // group is the database configuration group name of the current DAO.
	columns  YoubanTwoWayBotCooperationApplicationChannelColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                                  // handlers for customized model modification.
}

// YoubanTwoWayBotCooperationApplicationChannelColumns defines and stores column names for the table hg_youban_two_way_bot_cooperation_application_channel.
type YoubanTwoWayBotCooperationApplicationChannelColumns struct {
	Id            string //
	TenantId      string //
	ApplicationId string //
	ChannelId     string //
	Status        string //
	ErrorMessage  string //
	RetryCount    string //
	JoinedAt      string //
	CreatedAt     string //
	UpdatedAt     string //
}

// youbanTwoWayBotCooperationApplicationChannelColumns holds the columns for the table hg_youban_two_way_bot_cooperation_application_channel.
var youbanTwoWayBotCooperationApplicationChannelColumns = YoubanTwoWayBotCooperationApplicationChannelColumns{
	Id:            "id",
	TenantId:      "tenant_id",
	ApplicationId: "application_id",
	ChannelId:     "channel_id",
	Status:        "status",
	ErrorMessage:  "error_message",
	RetryCount:    "retry_count",
	JoinedAt:      "joined_at",
	CreatedAt:     "created_at",
	UpdatedAt:     "updated_at",
}

// NewYoubanTwoWayBotCooperationApplicationChannelDao creates and returns a new DAO object for table data access.
func NewYoubanTwoWayBotCooperationApplicationChannelDao(handlers ...gdb.ModelHandler) *YoubanTwoWayBotCooperationApplicationChannelDao {
	return &YoubanTwoWayBotCooperationApplicationChannelDao{
		group:    "default",
		table:    "hg_youban_two_way_bot_cooperation_application_channel",
		columns:  youbanTwoWayBotCooperationApplicationChannelColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanTwoWayBotCooperationApplicationChannelDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanTwoWayBotCooperationApplicationChannelDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanTwoWayBotCooperationApplicationChannelDao) Columns() YoubanTwoWayBotCooperationApplicationChannelColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanTwoWayBotCooperationApplicationChannelDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanTwoWayBotCooperationApplicationChannelDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanTwoWayBotCooperationApplicationChannelDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
