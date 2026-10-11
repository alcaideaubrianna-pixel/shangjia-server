// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectBotChannelDao is the data access object for the table hg_youban_publish_collect_bot_channel.
type YoubanPublishCollectBotChannelDao struct {
	table    string                                // table is the underlying table name of the DAO.
	group    string                                // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectBotChannelColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                    // handlers for customized model modification.
}

// YoubanPublishCollectBotChannelColumns defines and stores column names for the table hg_youban_publish_collect_bot_channel.
type YoubanPublishCollectBotChannelColumns struct {
	Id            string //
	TenantId      string //
	BotId         string //
	ChannelId     string //
	Status        string //
	BotRole       string //
	FirstSeenAt   string //
	LastSeenAt    string //
	RemovedAt     string //
	RemovedReason string //
	LastUpdateId  string //
	CreatedAt     string //
	UpdatedAt     string //
	DeletedAt     string //
}

// youbanPublishCollectBotChannelColumns holds the columns for the table hg_youban_publish_collect_bot_channel.
var youbanPublishCollectBotChannelColumns = YoubanPublishCollectBotChannelColumns{
	Id:            "id",
	TenantId:      "tenant_id",
	BotId:         "bot_id",
	ChannelId:     "channel_id",
	Status:        "status",
	BotRole:       "bot_role",
	FirstSeenAt:   "first_seen_at",
	LastSeenAt:    "last_seen_at",
	RemovedAt:     "removed_at",
	RemovedReason: "removed_reason",
	LastUpdateId:  "last_update_id",
	CreatedAt:     "created_at",
	UpdatedAt:     "updated_at",
	DeletedAt:     "deleted_at",
}

// NewYoubanPublishCollectBotChannelDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectBotChannelDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectBotChannelDao {
	return &YoubanPublishCollectBotChannelDao{
		group:    "default",
		table:    "hg_youban_publish_collect_bot_channel",
		columns:  youbanPublishCollectBotChannelColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectBotChannelDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectBotChannelDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectBotChannelDao) Columns() YoubanPublishCollectBotChannelColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectBotChannelDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectBotChannelDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectBotChannelDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
