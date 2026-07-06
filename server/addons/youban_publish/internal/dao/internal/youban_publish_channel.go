// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishChannelDao is the data access object for the table hg_youban_publish_channel.
type YoubanPublishChannelDao struct {
	table    string                      // table is the underlying table name of the DAO.
	group    string                      // group is the database configuration group name of the current DAO.
	columns  YoubanPublishChannelColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler          // handlers for customized model modification.
}

// YoubanPublishChannelColumns defines and stores column names for the table hg_youban_publish_channel.
type YoubanPublishChannelColumns struct {
	Id                  string // 主键
	TenantId            string // 租户ID
	MerchantId          string // 兼容旧版本商家ID
	TgAccountId         string // TG账号ID
	ChannelTitle        string // 频道名称
	ChannelUsername     string // 频道用户名
	TargetChatId        string // 目标Chat ID
	PublishDirection    string // 上架/下架频道
	CyclePublishEnabled string // 是否循环上架
	CyclePublishDays    string // 循环上架天数
	CyclePublishTime    string // 循环上架时间
	IsDefaultSelected   string // 是否默认选中
	BotIdJson           string // 绑定Bot ID JSON
	Remark              string // 备注
	Status              string // 状态
	LastRefreshStatus   string // 最近刷新状态
	LastRefreshMessage  string // 最近刷新信息
	LastRefreshAt       string // 最近刷新时间
	CreatedBy           string // 创建人
	UpdatedBy           string // 更新人
	DeletedBy           string // 删除人
	CreatedAt           string // 创建时间
	UpdatedAt           string // 更新时间
	DeletedAt           string // 删除时间
}

// youbanPublishChannelColumns holds the columns for the table hg_youban_publish_channel.
var youbanPublishChannelColumns = YoubanPublishChannelColumns{
	Id:                  "id",
	TenantId:            "tenant_id",
	MerchantId:          "merchant_id",
	TgAccountId:         "tg_account_id",
	ChannelTitle:        "channel_title",
	ChannelUsername:     "channel_username",
	TargetChatId:        "target_chat_id",
	PublishDirection:    "publish_direction",
	CyclePublishEnabled: "cycle_publish_enabled",
	CyclePublishDays:    "cycle_publish_days",
	CyclePublishTime:    "cycle_publish_time",
	IsDefaultSelected:   "is_default_selected",
	BotIdJson:           "bot_id_json",
	Remark:              "remark",
	Status:              "status",
	LastRefreshStatus:   "last_refresh_status",
	LastRefreshMessage:  "last_refresh_message",
	LastRefreshAt:       "last_refresh_at",
	CreatedBy:           "created_by",
	UpdatedBy:           "updated_by",
	DeletedBy:           "deleted_by",
	CreatedAt:           "created_at",
	UpdatedAt:           "updated_at",
	DeletedAt:           "deleted_at",
}

// NewYoubanPublishChannelDao creates and returns a new DAO object for table data access.
func NewYoubanPublishChannelDao(handlers ...gdb.ModelHandler) *YoubanPublishChannelDao {
	return &YoubanPublishChannelDao{
		group:    "default",
		table:    "hg_youban_publish_channel",
		columns:  youbanPublishChannelColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishChannelDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishChannelDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishChannelDao) Columns() YoubanPublishChannelColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishChannelDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishChannelDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishChannelDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
