// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanTwoWayBotMessageDao is the data access object for the table hg_youban_two_way_bot_message.
type YoubanTwoWayBotMessageDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanTwoWayBotMessageColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanTwoWayBotMessageColumns defines and stores column names for the table hg_youban_two_way_bot_message.
type YoubanTwoWayBotMessageColumns struct {
	Id              string //
	TenantId        string //
	BotId           string //
	Direction       string //
	TelegramUserId  string //
	ThreadId        string //
	SourceChatId    string //
	SourceMessageId string //
	TargetChatId    string //
	TargetMessageId string //
	MediaGroupId    string //
	Status          string //
	ErrorMessage    string //
	CreatedAt       string //
	UpdatedAt       string //
}

// youbanTwoWayBotMessageColumns holds the columns for the table hg_youban_two_way_bot_message.
var youbanTwoWayBotMessageColumns = YoubanTwoWayBotMessageColumns{
	Id:              "id",
	TenantId:        "tenant_id",
	BotId:           "bot_id",
	Direction:       "direction",
	TelegramUserId:  "telegram_user_id",
	ThreadId:        "thread_id",
	SourceChatId:    "source_chat_id",
	SourceMessageId: "source_message_id",
	TargetChatId:    "target_chat_id",
	TargetMessageId: "target_message_id",
	MediaGroupId:    "media_group_id",
	Status:          "status",
	ErrorMessage:    "error_message",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
}

// NewYoubanTwoWayBotMessageDao creates and returns a new DAO object for table data access.
func NewYoubanTwoWayBotMessageDao(handlers ...gdb.ModelHandler) *YoubanTwoWayBotMessageDao {
	return &YoubanTwoWayBotMessageDao{
		group:    "default",
		table:    "hg_youban_two_way_bot_message",
		columns:  youbanTwoWayBotMessageColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanTwoWayBotMessageDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanTwoWayBotMessageDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanTwoWayBotMessageDao) Columns() YoubanTwoWayBotMessageColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanTwoWayBotMessageDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanTwoWayBotMessageDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanTwoWayBotMessageDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
