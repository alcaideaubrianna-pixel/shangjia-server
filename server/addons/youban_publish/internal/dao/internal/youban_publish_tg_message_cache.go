// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgMessageCacheDao is the data access object for the table hg_youban_publish_tg_message_cache.
type YoubanPublishTgMessageCacheDao struct {
	table    string                             // table is the underlying table name of the DAO.
	group    string                             // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgMessageCacheColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                 // handlers for customized model modification.
}

// YoubanPublishTgMessageCacheColumns defines and stores column names for the table hg_youban_publish_tg_message_cache.
type YoubanPublishTgMessageCacheColumns struct {
	Id           string //
	TenantId     string //
	TgAccountId  string //
	ChannelId    string //
	TargetChatId string //
	TgMessageId  string //
	MessageText  string //
	MessageDate  string //
	MediaGroupId string //
	CreatedAt    string //
	UpdatedAt    string //
}

// youbanPublishTgMessageCacheColumns holds the columns for the table hg_youban_publish_tg_message_cache.
var youbanPublishTgMessageCacheColumns = YoubanPublishTgMessageCacheColumns{
	Id:           "id",
	TenantId:     "tenant_id",
	TgAccountId:  "tg_account_id",
	ChannelId:    "channel_id",
	TargetChatId: "target_chat_id",
	TgMessageId:  "tg_message_id",
	MessageText:  "message_text",
	MessageDate:  "message_date",
	MediaGroupId: "media_group_id",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
}

// NewYoubanPublishTgMessageCacheDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgMessageCacheDao(handlers ...gdb.ModelHandler) *YoubanPublishTgMessageCacheDao {
	return &YoubanPublishTgMessageCacheDao{
		group:    "default",
		table:    "hg_youban_publish_tg_message_cache",
		columns:  youbanPublishTgMessageCacheColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgMessageCacheDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgMessageCacheDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgMessageCacheDao) Columns() YoubanPublishTgMessageCacheColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgMessageCacheDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgMessageCacheDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgMessageCacheDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
