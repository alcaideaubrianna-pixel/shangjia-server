// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// AddonLazysheepTggoBindingDao is the data access object for the table hg_addon_lazysheep_tggo_binding.
type AddonLazysheepTggoBindingDao struct {
	table    string                           // table is the underlying table name of the DAO.
	group    string                           // group is the database configuration group name of the current DAO.
	columns  AddonLazysheepTggoBindingColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler               // handlers for customized model modification.
}

// AddonLazysheepTggoBindingColumns defines and stores column names for the table hg_addon_lazysheep_tggo_binding.
type AddonLazysheepTggoBindingColumns struct {
	Id              string // 主键
	BindingKey      string // 绑定标识
	BotId           string // 机器人ID
	BotKey          string // 机器人标识
	SourceUrl       string // BangChat 链接
	SourceToken     string // BangChat token
	SourceRoomId    string // 来源房间ID
	SourcePairId    string // 来源 pairId
	ReviewChatId    string // 审核群ID
	PublishChatId   string // 推送频道ID
	AutoPush        string // 自动推送
	ReviewEnabled   string // 审核开关
	PublishEnabled  string // 推送开关
	VerifyEnabled   string // 验证按钮开关
	LocationEnabled string // 位置按钮开关
	PluginSettings  string // 插件状态
	LastPullId      string // 最后拉取ID
	LastCursor      string // 最后游标
	Status          string // 状态
	CreatedBy       string // 创建者
	UpdatedBy       string // 更新者
	CreatedAt       string // 创建时间
	UpdatedAt       string // 更新时间
}

// addonLazysheepTggoBindingColumns holds the columns for the table hg_addon_lazysheep_tggo_binding.
var addonLazysheepTggoBindingColumns = AddonLazysheepTggoBindingColumns{
	Id:              "id",
	BindingKey:      "binding_key",
	BotId:           "bot_id",
	BotKey:          "bot_key",
	SourceUrl:       "source_url",
	SourceToken:     "source_token",
	SourceRoomId:    "source_room_id",
	SourcePairId:    "source_pair_id",
	ReviewChatId:    "review_chat_id",
	PublishChatId:   "publish_chat_id",
	AutoPush:        "auto_push",
	ReviewEnabled:   "review_enabled",
	PublishEnabled:  "publish_enabled",
	VerifyEnabled:   "verify_enabled",
	LocationEnabled: "location_enabled",
	PluginSettings:  "plugin_settings",
	LastPullId:      "last_pull_id",
	LastCursor:      "last_cursor",
	Status:          "status",
	CreatedBy:       "created_by",
	UpdatedBy:       "updated_by",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
}

// NewAddonLazysheepTggoBindingDao creates and returns a new DAO object for table data access.
func NewAddonLazysheepTggoBindingDao(handlers ...gdb.ModelHandler) *AddonLazysheepTggoBindingDao {
	return &AddonLazysheepTggoBindingDao{
		group:    "default",
		table:    "hg_addon_lazysheep_tggo_binding",
		columns:  addonLazysheepTggoBindingColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AddonLazysheepTggoBindingDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AddonLazysheepTggoBindingDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AddonLazysheepTggoBindingDao) Columns() AddonLazysheepTggoBindingColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AddonLazysheepTggoBindingDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AddonLazysheepTggoBindingDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AddonLazysheepTggoBindingDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
