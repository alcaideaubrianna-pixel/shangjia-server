// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotRelayDeliveryDao is the data access object for the table hg_youban_bot_relay_delivery.
type YoubanBotRelayDeliveryDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanBotRelayDeliveryColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanBotRelayDeliveryColumns defines and stores column names for the table hg_youban_bot_relay_delivery.
type YoubanBotRelayDeliveryColumns struct {
	Id              string //
	BotId           string //
	BindingId       string //
	SourceChatId    string //
	SourceMessageId string //
	MediaGroupId    string //
	TargetChatId    string //
	Kind            string //
	PairKey         string //
	TopicTitle      string //
	RawJson         string //
	Status          string //
	Attempts        string //
	AvailableAt     string //
	LeaseToken      string //
	TargetMessageId string //
	ErrorMessage    string //
	CreatedAt       string //
	UpdatedAt       string //
}

// youbanBotRelayDeliveryColumns holds the columns for the table hg_youban_bot_relay_delivery.
var youbanBotRelayDeliveryColumns = YoubanBotRelayDeliveryColumns{
	Id:              "id",
	BotId:           "bot_id",
	BindingId:       "binding_id",
	SourceChatId:    "source_chat_id",
	SourceMessageId: "source_message_id",
	MediaGroupId:    "media_group_id",
	TargetChatId:    "target_chat_id",
	Kind:            "kind",
	PairKey:         "pair_key",
	TopicTitle:      "topic_title",
	RawJson:         "raw_json",
	Status:          "status",
	Attempts:        "attempts",
	AvailableAt:     "available_at",
	LeaseToken:      "lease_token",
	TargetMessageId: "target_message_id",
	ErrorMessage:    "error_message",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
}

// NewYoubanBotRelayDeliveryDao creates and returns a new DAO object for table data access.
func NewYoubanBotRelayDeliveryDao(handlers ...gdb.ModelHandler) *YoubanBotRelayDeliveryDao {
	return &YoubanBotRelayDeliveryDao{
		group:    "default",
		table:    "hg_youban_bot_relay_delivery",
		columns:  youbanBotRelayDeliveryColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotRelayDeliveryDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotRelayDeliveryDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotRelayDeliveryDao) Columns() YoubanBotRelayDeliveryColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotRelayDeliveryDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotRelayDeliveryDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotRelayDeliveryDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
