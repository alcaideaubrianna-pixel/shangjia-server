// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotRelayAuditTopicDao is the data access object for the table hg_youban_bot_relay_audit_topic.
type YoubanBotRelayAuditTopicDao struct {
	table    string                          // table is the underlying table name of the DAO.
	group    string                          // group is the database configuration group name of the current DAO.
	columns  YoubanBotRelayAuditTopicColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler              // handlers for customized model modification.
}

// YoubanBotRelayAuditTopicColumns defines and stores column names for the table hg_youban_bot_relay_audit_topic.
type YoubanBotRelayAuditTopicColumns struct {
	Id        string //
	BotId     string //
	GroupId   string //
	PairKey   string //
	ThreadId  string //
	CreatedAt string //
	UpdatedAt string //
}

// youbanBotRelayAuditTopicColumns holds the columns for the table hg_youban_bot_relay_audit_topic.
var youbanBotRelayAuditTopicColumns = YoubanBotRelayAuditTopicColumns{
	Id:        "id",
	BotId:     "bot_id",
	GroupId:   "group_id",
	PairKey:   "pair_key",
	ThreadId:  "thread_id",
	CreatedAt: "created_at",
	UpdatedAt: "updated_at",
}

// NewYoubanBotRelayAuditTopicDao creates and returns a new DAO object for table data access.
func NewYoubanBotRelayAuditTopicDao(handlers ...gdb.ModelHandler) *YoubanBotRelayAuditTopicDao {
	return &YoubanBotRelayAuditTopicDao{
		group:    "default",
		table:    "hg_youban_bot_relay_audit_topic",
		columns:  youbanBotRelayAuditTopicColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotRelayAuditTopicDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotRelayAuditTopicDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotRelayAuditTopicDao) Columns() YoubanBotRelayAuditTopicColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotRelayAuditTopicDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotRelayAuditTopicDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotRelayAuditTopicDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
