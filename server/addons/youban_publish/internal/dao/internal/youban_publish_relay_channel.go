// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishRelayChannelDao is the data access object for the table hg_youban_publish_relay_channel.
type YoubanPublishRelayChannelDao struct {
	table    string                           // table is the underlying table name of the DAO.
	group    string                           // group is the database configuration group name of the current DAO.
	columns  YoubanPublishRelayChannelColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler               // handlers for customized model modification.
}

// YoubanPublishRelayChannelColumns defines and stores column names for the table hg_youban_publish_relay_channel.
type YoubanPublishRelayChannelColumns struct {
	Id                  string //
	TenantId            string //
	TgAccountId         string //
	ChannelId           string //
	TargetChatId        string //
	Title               string //
	Status              string //
	HealthStatus        string //
	BotPermissionStatus string //
	CooldownUntil       string //
	LastError           string //
	InFlightCount       string //
	LastUsedAt          string //
	CreatedAt           string //
	UpdatedAt           string //
	DeletedAt           string //
}

// youbanPublishRelayChannelColumns holds the columns for the table hg_youban_publish_relay_channel.
var youbanPublishRelayChannelColumns = YoubanPublishRelayChannelColumns{
	Id:                  "id",
	TenantId:            "tenant_id",
	TgAccountId:         "tg_account_id",
	ChannelId:           "channel_id",
	TargetChatId:        "target_chat_id",
	Title:               "title",
	Status:              "status",
	HealthStatus:        "health_status",
	BotPermissionStatus: "bot_permission_status",
	CooldownUntil:       "cooldown_until",
	LastError:           "last_error",
	InFlightCount:       "in_flight_count",
	LastUsedAt:          "last_used_at",
	CreatedAt:           "created_at",
	UpdatedAt:           "updated_at",
	DeletedAt:           "deleted_at",
}

// NewYoubanPublishRelayChannelDao creates and returns a new DAO object for table data access.
func NewYoubanPublishRelayChannelDao(handlers ...gdb.ModelHandler) *YoubanPublishRelayChannelDao {
	return &YoubanPublishRelayChannelDao{
		group:    "default",
		table:    "hg_youban_publish_relay_channel",
		columns:  youbanPublishRelayChannelColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishRelayChannelDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishRelayChannelDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishRelayChannelDao) Columns() YoubanPublishRelayChannelColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishRelayChannelDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishRelayChannelDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishRelayChannelDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
