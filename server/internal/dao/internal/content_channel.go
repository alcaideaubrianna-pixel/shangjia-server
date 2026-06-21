// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// ContentChannelDao is the data access object for the table hg_content_channel.
type ContentChannelDao struct {
	table    string                // table is the underlying table name of the DAO.
	group    string                // group is the database configuration group name of the current DAO.
	columns  ContentChannelColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler    // handlers for customized model modification.
}

// ContentChannelColumns defines and stores column names for the table hg_content_channel.
type ContentChannelColumns struct {
	Id              string // ID
	SourceChannelId string // FeiNiu频道ID
	TgChatId        string // TG Chat ID
	Title           string // 频道标题
	Username        string // 频道用户名
	InviteLink      string // 邀请链接
	SourceType      string // 来源类型
	PublicStatus    string // 前台公开状态
	AuthStatus      string // 授权状态
	Remark          string // 备注
	Status          string // 状态
	CreatedAt       string // 创建时间
	UpdatedAt       string // 更新时间
	DeletedAt       string // 删除时间
}

// contentChannelColumns holds the columns for the table hg_content_channel.
var contentChannelColumns = ContentChannelColumns{
	Id:              "id",
	SourceChannelId: "source_channel_id",
	TgChatId:        "tg_chat_id",
	Title:           "title",
	Username:        "username",
	InviteLink:      "invite_link",
	SourceType:      "source_type",
	PublicStatus:    "public_status",
	AuthStatus:      "auth_status",
	Remark:          "remark",
	Status:          "status",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
	DeletedAt:       "deleted_at",
}

// NewContentChannelDao creates and returns a new DAO object for table data access.
func NewContentChannelDao(handlers ...gdb.ModelHandler) *ContentChannelDao {
	return &ContentChannelDao{
		group:    "default",
		table:    "hg_content_channel",
		columns:  contentChannelColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *ContentChannelDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *ContentChannelDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *ContentChannelDao) Columns() ContentChannelColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *ContentChannelDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *ContentChannelDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *ContentChannelDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
