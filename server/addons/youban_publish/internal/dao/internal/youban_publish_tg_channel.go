// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgChannelDao is the data access object for the table hg_youban_publish_tg_channel.
type YoubanPublishTgChannelDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgChannelColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanPublishTgChannelColumns defines and stores column names for the table hg_youban_publish_tg_channel.
type YoubanPublishTgChannelColumns struct {
	Id              string // 主键
	TenantId        string // 租户ID
	MerchantId      string // 兼容旧版本商家ID
	TgAccountId     string // TG账号ID
	ChannelId       string // 频道ID
	AccessHash      string // AccessHash
	ChannelTitle    string // 频道名称
	ChannelUsername string // 频道用户名
	IsBroadcast     string // 是否频道
	IsMegagroup     string // 是否群组
	CanPostMessages string // 账号可发频道消息
	CanInviteUsers  string // 账号可邀请用户
	CanAddAdmins    string // 账号可添加管理员
	LastSyncAt      string // 最后同步时间
	CreatedAt       string // 创建时间
	UpdatedAt       string // 更新时间
}

// youbanPublishTgChannelColumns holds the columns for the table hg_youban_publish_tg_channel.
var youbanPublishTgChannelColumns = YoubanPublishTgChannelColumns{
	Id:              "id",
	TenantId:        "tenant_id",
	MerchantId:      "merchant_id",
	TgAccountId:     "tg_account_id",
	ChannelId:       "channel_id",
	AccessHash:      "access_hash",
	ChannelTitle:    "channel_title",
	ChannelUsername: "channel_username",
	IsBroadcast:     "is_broadcast",
	IsMegagroup:     "is_megagroup",
	CanPostMessages: "can_post_messages",
	CanInviteUsers:  "can_invite_users",
	CanAddAdmins:    "can_add_admins",
	LastSyncAt:      "last_sync_at",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
}

// NewYoubanPublishTgChannelDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgChannelDao(handlers ...gdb.ModelHandler) *YoubanPublishTgChannelDao {
	return &YoubanPublishTgChannelDao{
		group:    "default",
		table:    "hg_youban_publish_tg_channel",
		columns:  youbanPublishTgChannelColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgChannelDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgChannelDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgChannelDao) Columns() YoubanPublishTgChannelColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgChannelDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgChannelDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgChannelDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
