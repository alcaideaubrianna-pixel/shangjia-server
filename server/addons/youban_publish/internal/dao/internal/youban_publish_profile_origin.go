// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishProfileOriginDao is the data access object for the table hg_youban_publish_profile_origin.
type YoubanPublishProfileOriginDao struct {
	table    string                            // table is the underlying table name of the DAO.
	group    string                            // group is the database configuration group name of the current DAO.
	columns  YoubanPublishProfileOriginColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                // handlers for customized model modification.
}

// YoubanPublishProfileOriginColumns defines and stores column names for the table hg_youban_publish_profile_origin.
type YoubanPublishProfileOriginColumns struct {
	Id              string //
	TenantId        string //
	AccountId       string //
	ProfileId       string //
	OriginKey       string //
	OriginKind      string //
	CollectMethod   string //
	SourceId        string //
	EventId         string //
	BotId           string //
	TgAccountId     string //
	FollowAccountId string //
	ChatIdRaw       string //
	ChatId          string //
	MessageId       string //
	GroupedId       string //
	ChannelTitle    string //
	ChannelUsername string //
	ReceivedAt      string //
	EvidenceKind    string //
	Completeness    string //
	CreatedAt       string //
}

// youbanPublishProfileOriginColumns holds the columns for the table hg_youban_publish_profile_origin.
var youbanPublishProfileOriginColumns = YoubanPublishProfileOriginColumns{
	Id:              "id",
	TenantId:        "tenant_id",
	AccountId:       "account_id",
	ProfileId:       "profile_id",
	OriginKey:       "origin_key",
	OriginKind:      "origin_kind",
	CollectMethod:   "collect_method",
	SourceId:        "source_id",
	EventId:         "event_id",
	BotId:           "bot_id",
	TgAccountId:     "tg_account_id",
	FollowAccountId: "follow_account_id",
	ChatIdRaw:       "chat_id_raw",
	ChatId:          "chat_id",
	MessageId:       "message_id",
	GroupedId:       "grouped_id",
	ChannelTitle:    "channel_title",
	ChannelUsername: "channel_username",
	ReceivedAt:      "received_at",
	EvidenceKind:    "evidence_kind",
	Completeness:    "completeness",
	CreatedAt:       "created_at",
}

// NewYoubanPublishProfileOriginDao creates and returns a new DAO object for table data access.
func NewYoubanPublishProfileOriginDao(handlers ...gdb.ModelHandler) *YoubanPublishProfileOriginDao {
	return &YoubanPublishProfileOriginDao{
		group:    "default",
		table:    "hg_youban_publish_profile_origin",
		columns:  youbanPublishProfileOriginColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishProfileOriginDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishProfileOriginDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishProfileOriginDao) Columns() YoubanPublishProfileOriginColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishProfileOriginDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishProfileOriginDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishProfileOriginDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
