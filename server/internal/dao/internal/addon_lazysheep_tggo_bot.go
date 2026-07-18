// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// AddonLazysheepTggoBotDao is the data access object for the table hg_addon_lazysheep_tggo_bot.
type AddonLazysheepTggoBotDao struct {
	table    string                       // table is the underlying table name of the DAO.
	group    string                       // group is the database configuration group name of the current DAO.
	columns  AddonLazysheepTggoBotColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler           // handlers for customized model modification.
}

// AddonLazysheepTggoBotColumns defines and stores column names for the table hg_addon_lazysheep_tggo_bot.
type AddonLazysheepTggoBotColumns struct {
	Id            string // 主键
	BotKey        string // 机器人标识
	Role          string // 机器人角色
	MemberId      string // 所属后台用户
	Token         string // Telegram Bot Token
	BotName       string // 机器人名称
	Username      string // Telegram username
	WebhookSecret string // Webhook Secret
	WebhookPath   string // Webhook 路径
	Enabled       string // 是否启用
	AutoPull      string // 自动采集
	AutoForward   string // 自动推送
	ReviewEnabled string // 审核开关
	AllowVerify   string // 允许查看验证
	AllowLocation string // 允许查看位置
	MemberVerify  string // 验证仅会员
	MemberPoints  string // 积分解锁
	SignFollow    string // 签到关注校验
	SignChannels  string // 签到必关频道
	ReviewText    string // 审核文案模板
	PublishText   string // 推送文案模板
	Sort          string // 排序
	Status        string // 状态
	CreatedBy     string // 创建者
	UpdatedBy     string // 更新者
	CreatedAt     string // 创建时间
	UpdatedAt     string // 更新时间
}

// addonLazysheepTggoBotColumns holds the columns for the table hg_addon_lazysheep_tggo_bot.
var addonLazysheepTggoBotColumns = AddonLazysheepTggoBotColumns{
	Id:            "id",
	BotKey:        "bot_key",
	Role:          "role",
	MemberId:      "member_id",
	Token:         "token",
	BotName:       "bot_name",
	Username:      "username",
	WebhookSecret: "webhook_secret",
	WebhookPath:   "webhook_path",
	Enabled:       "enabled",
	AutoPull:      "auto_pull",
	AutoForward:   "auto_forward",
	ReviewEnabled: "review_enabled",
	AllowVerify:   "allow_verify",
	AllowLocation: "allow_location",
	MemberVerify:  "member_verify",
	MemberPoints:  "member_points",
	SignFollow:    "sign_follow",
	SignChannels:  "sign_channels",
	ReviewText:    "review_text",
	PublishText:   "publish_text",
	Sort:          "sort",
	Status:        "status",
	CreatedBy:     "created_by",
	UpdatedBy:     "updated_by",
	CreatedAt:     "created_at",
	UpdatedAt:     "updated_at",
}

// NewAddonLazysheepTggoBotDao creates and returns a new DAO object for table data access.
func NewAddonLazysheepTggoBotDao(handlers ...gdb.ModelHandler) *AddonLazysheepTggoBotDao {
	return &AddonLazysheepTggoBotDao{
		group:    "default",
		table:    "hg_addon_lazysheep_tggo_bot",
		columns:  addonLazysheepTggoBotColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AddonLazysheepTggoBotDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AddonLazysheepTggoBotDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AddonLazysheepTggoBotDao) Columns() AddonLazysheepTggoBotColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AddonLazysheepTggoBotDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AddonLazysheepTggoBotDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AddonLazysheepTggoBotDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
