// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectChannelDao is the data access object for the table hg_youban_publish_collect_channel.
type YoubanPublishCollectChannelDao struct {
	table    string                             // table is the underlying table name of the DAO.
	group    string                             // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectChannelColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                 // handlers for customized model modification.
}

// YoubanPublishCollectChannelColumns defines and stores column names for the table hg_youban_publish_collect_channel.
type YoubanPublishCollectChannelColumns struct {
	Id             string //
	TenantId       string //
	TelegramChatId string //
	ChatType       string //
	Title          string //
	Username       string //
	InviteLink     string //
	About          string //
	PhotoFileId    string //
	IsBroadcast    string //
	IsMegagroup    string //
	LastMessageId  string //
	LastMessageAt  string //
	LastSeenAt     string //
	Status         string //
	CreatedAt      string //
	UpdatedAt      string //
	DeletedAt      string //
}

// youbanPublishCollectChannelColumns holds the columns for the table hg_youban_publish_collect_channel.
var youbanPublishCollectChannelColumns = YoubanPublishCollectChannelColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	TelegramChatId: "telegram_chat_id",
	ChatType:       "chat_type",
	Title:          "title",
	Username:       "username",
	InviteLink:     "invite_link",
	About:          "about",
	PhotoFileId:    "photo_file_id",
	IsBroadcast:    "is_broadcast",
	IsMegagroup:    "is_megagroup",
	LastMessageId:  "last_message_id",
	LastMessageAt:  "last_message_at",
	LastSeenAt:     "last_seen_at",
	Status:         "status",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
	DeletedAt:      "deleted_at",
}

// NewYoubanPublishCollectChannelDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectChannelDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectChannelDao {
	return &YoubanPublishCollectChannelDao{
		group:    "default",
		table:    "hg_youban_publish_collect_channel",
		columns:  youbanPublishCollectChannelColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectChannelDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectChannelDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectChannelDao) Columns() YoubanPublishCollectChannelColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectChannelDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectChannelDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectChannelDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
