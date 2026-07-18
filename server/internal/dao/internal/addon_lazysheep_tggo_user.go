// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// AddonLazysheepTggoUserDao is the data access object for the table hg_addon_lazysheep_tggo_user.
type AddonLazysheepTggoUserDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  AddonLazysheepTggoUserColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// AddonLazysheepTggoUserColumns defines and stores column names for the table hg_addon_lazysheep_tggo_user.
type AddonLazysheepTggoUserColumns struct {
	Id           string // 主键
	TelegramId   string // Telegram 用户ID
	BotKey       string // 机器人标识
	MemberId     string // 后台用户ID
	Username     string // 用户名
	FirstName    string // 名
	LastName     string // 姓
	LanguageCode string // 语言
	IsBot        string // 是否机器人
	MemberLevel  string // 会员等级
	Points       string // 积分
	LastActiveAt string // 最后活跃时间
	Status       string // 状态
	CreatedAt    string // 创建时间
	UpdatedAt    string // 更新时间
}

// addonLazysheepTggoUserColumns holds the columns for the table hg_addon_lazysheep_tggo_user.
var addonLazysheepTggoUserColumns = AddonLazysheepTggoUserColumns{
	Id:           "id",
	TelegramId:   "telegram_id",
	BotKey:       "bot_key",
	MemberId:     "member_id",
	Username:     "username",
	FirstName:    "first_name",
	LastName:     "last_name",
	LanguageCode: "language_code",
	IsBot:        "is_bot",
	MemberLevel:  "member_level",
	Points:       "points",
	LastActiveAt: "last_active_at",
	Status:       "status",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
}

// NewAddonLazysheepTggoUserDao creates and returns a new DAO object for table data access.
func NewAddonLazysheepTggoUserDao(handlers ...gdb.ModelHandler) *AddonLazysheepTggoUserDao {
	return &AddonLazysheepTggoUserDao{
		group:    "default",
		table:    "hg_addon_lazysheep_tggo_user",
		columns:  addonLazysheepTggoUserColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AddonLazysheepTggoUserDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AddonLazysheepTggoUserDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AddonLazysheepTggoUserDao) Columns() AddonLazysheepTggoUserColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AddonLazysheepTggoUserDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AddonLazysheepTggoUserDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AddonLazysheepTggoUserDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
