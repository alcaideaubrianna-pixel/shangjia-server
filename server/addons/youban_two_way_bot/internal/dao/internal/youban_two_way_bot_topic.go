// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanTwoWayBotTopicDao is the data access object for the table hg_youban_two_way_bot_topic.
type YoubanTwoWayBotTopicDao struct {
	table    string                      // table is the underlying table name of the DAO.
	group    string                      // group is the database configuration group name of the current DAO.
	columns  YoubanTwoWayBotTopicColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler          // handlers for customized model modification.
}

// YoubanTwoWayBotTopicColumns defines and stores column names for the table hg_youban_two_way_bot_topic.
type YoubanTwoWayBotTopicColumns struct {
	Id                string //
	TenantId          string //
	BotId             string //
	TelegramUserId    string //
	TelegramUsername  string //
	TelegramFirstName string //
	TelegramLastName  string //
	ThreadId          string //
	Title             string //
	Closed            string //
	LastMessageAt     string //
	CreatedAt         string //
	UpdatedAt         string //
	DeletedAt         string //
}

// youbanTwoWayBotTopicColumns holds the columns for the table hg_youban_two_way_bot_topic.
var youbanTwoWayBotTopicColumns = YoubanTwoWayBotTopicColumns{
	Id:                "id",
	TenantId:          "tenant_id",
	BotId:             "bot_id",
	TelegramUserId:    "telegram_user_id",
	TelegramUsername:  "telegram_username",
	TelegramFirstName: "telegram_first_name",
	TelegramLastName:  "telegram_last_name",
	ThreadId:          "thread_id",
	Title:             "title",
	Closed:            "closed",
	LastMessageAt:     "last_message_at",
	CreatedAt:         "created_at",
	UpdatedAt:         "updated_at",
	DeletedAt:         "deleted_at",
}

// NewYoubanTwoWayBotTopicDao creates and returns a new DAO object for table data access.
func NewYoubanTwoWayBotTopicDao(handlers ...gdb.ModelHandler) *YoubanTwoWayBotTopicDao {
	return &YoubanTwoWayBotTopicDao{
		group:    "default",
		table:    "hg_youban_two_way_bot_topic",
		columns:  youbanTwoWayBotTopicColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanTwoWayBotTopicDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanTwoWayBotTopicDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanTwoWayBotTopicDao) Columns() YoubanTwoWayBotTopicColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanTwoWayBotTopicDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanTwoWayBotTopicDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanTwoWayBotTopicDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
