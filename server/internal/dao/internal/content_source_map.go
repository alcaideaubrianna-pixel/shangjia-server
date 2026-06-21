// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// ContentSourceMapDao is the data access object for the table hg_content_source_map.
type ContentSourceMapDao struct {
	table    string                  // table is the underlying table name of the DAO.
	group    string                  // group is the database configuration group name of the current DAO.
	columns  ContentSourceMapColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler      // handlers for customized model modification.
}

// ContentSourceMapColumns defines and stores column names for the table hg_content_source_map.
type ContentSourceMapColumns struct {
	Id              string // ID
	ProfileId       string // 资料ID
	SourceType      string // 来源类型
	SourceKey       string // 来源唯一键
	SourceChannelId string // 来源频道ID
	SourceMessageId string // 来源消息ID
	SourceGroupedId string // 来源媒体组ID
	SourceTextHash  string // 来源文本哈希
	RawText         string // 原始文本
	RawMessageJson  string // 原始消息JSON
	CreatedAt       string // 创建时间
}

// contentSourceMapColumns holds the columns for the table hg_content_source_map.
var contentSourceMapColumns = ContentSourceMapColumns{
	Id:              "id",
	ProfileId:       "profile_id",
	SourceType:      "source_type",
	SourceKey:       "source_key",
	SourceChannelId: "source_channel_id",
	SourceMessageId: "source_message_id",
	SourceGroupedId: "source_grouped_id",
	SourceTextHash:  "source_text_hash",
	RawText:         "raw_text",
	RawMessageJson:  "raw_message_json",
	CreatedAt:       "created_at",
}

// NewContentSourceMapDao creates and returns a new DAO object for table data access.
func NewContentSourceMapDao(handlers ...gdb.ModelHandler) *ContentSourceMapDao {
	return &ContentSourceMapDao{
		group:    "default",
		table:    "hg_content_source_map",
		columns:  contentSourceMapColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *ContentSourceMapDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *ContentSourceMapDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *ContentSourceMapDao) Columns() ContentSourceMapColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *ContentSourceMapDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *ContentSourceMapDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *ContentSourceMapDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
