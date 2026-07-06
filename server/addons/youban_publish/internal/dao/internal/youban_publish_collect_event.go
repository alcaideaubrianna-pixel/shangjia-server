// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectEventDao is the data access object for the table hg_youban_publish_collect_event.
type YoubanPublishCollectEventDao struct {
	table    string                           // table is the underlying table name of the DAO.
	group    string                           // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectEventColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler               // handlers for customized model modification.
}

// YoubanPublishCollectEventColumns defines and stores column names for the table hg_youban_publish_collect_event.
type YoubanPublishCollectEventColumns struct {
	Id              string // 主键
	TenantId        string // 租户ID
	AccountId       string // 所属账号ID
	SourceId        string // 采集源ID
	SourceType      string // 来源类型
	BotId           string // 机器人ID
	TgAccountId     string // 协议号ID
	SourceChatId    string // 来源频道/群聊ID
	SourceMessageId string // 来源消息ID
	SourceGroupedId string // 媒体组ID
	SourceUniqueKey string // 来源唯一键
	RawText         string // 原始文本
	MediaCount      string // 媒体数量
	MediaJson       string // 媒体JSON
	TextHash        string // 文本哈希
	DedupeKey       string // 去重键
	Status          string // 状态
	ErrorMessage    string // 错误信息
	ReceivedAt      string // 接收时间
	ProcessedAt     string // 处理时间
	CreatedAt       string // 创建时间
	UpdatedAt       string // 更新时间
}

// youbanPublishCollectEventColumns holds the columns for the table hg_youban_publish_collect_event.
var youbanPublishCollectEventColumns = YoubanPublishCollectEventColumns{
	Id:              "id",
	TenantId:        "tenant_id",
	AccountId:       "account_id",
	SourceId:        "source_id",
	SourceType:      "source_type",
	BotId:           "bot_id",
	TgAccountId:     "tg_account_id",
	SourceChatId:    "source_chat_id",
	SourceMessageId: "source_message_id",
	SourceGroupedId: "source_grouped_id",
	SourceUniqueKey: "source_unique_key",
	RawText:         "raw_text",
	MediaCount:      "media_count",
	MediaJson:       "media_json",
	TextHash:        "text_hash",
	DedupeKey:       "dedupe_key",
	Status:          "status",
	ErrorMessage:    "error_message",
	ReceivedAt:      "received_at",
	ProcessedAt:     "processed_at",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
}

// NewYoubanPublishCollectEventDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectEventDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectEventDao {
	return &YoubanPublishCollectEventDao{
		group:    "default",
		table:    "hg_youban_publish_collect_event",
		columns:  youbanPublishCollectEventColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectEventDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectEventDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectEventDao) Columns() YoubanPublishCollectEventColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectEventDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectEventDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectEventDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
