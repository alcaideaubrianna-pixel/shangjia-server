// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishBotDao is the data access object for the table hg_youban_publish_bot.
type YoubanPublishBotDao struct {
	table    string                  // table is the underlying table name of the DAO.
	group    string                  // group is the database configuration group name of the current DAO.
	columns  YoubanPublishBotColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler      // handlers for customized model modification.
}

// YoubanPublishBotColumns defines and stores column names for the table hg_youban_publish_bot.
type YoubanPublishBotColumns struct {
	Id          string // 主键
	BotName     string // Bot名称
	BotUsername string // Bot用户名
	BotToken    string // Bot Token
	Remark      string // 备注
	Status      string // 状态
	CreatedBy   string // 创建人
	UpdatedBy   string // 更新人
	DeletedBy   string // 删除人
	CreatedAt   string // 创建时间
	UpdatedAt   string // 更新时间
	DeletedAt   string // 删除时间
}

// youbanPublishBotColumns holds the columns for the table hg_youban_publish_bot.
var youbanPublishBotColumns = YoubanPublishBotColumns{
	Id:          "id",
	BotName:     "bot_name",
	BotUsername: "bot_username",
	BotToken:    "bot_token",
	Remark:      "remark",
	Status:      "status",
	CreatedBy:   "created_by",
	UpdatedBy:   "updated_by",
	DeletedBy:   "deleted_by",
	CreatedAt:   "created_at",
	UpdatedAt:   "updated_at",
	DeletedAt:   "deleted_at",
}

// NewYoubanPublishBotDao creates and returns a new DAO object for table data access.
func NewYoubanPublishBotDao(handlers ...gdb.ModelHandler) *YoubanPublishBotDao {
	return &YoubanPublishBotDao{
		group:    "default",
		table:    "hg_youban_publish_bot",
		columns:  youbanPublishBotColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishBotDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishBotDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishBotDao) Columns() YoubanPublishBotColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishBotDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishBotDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishBotDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
