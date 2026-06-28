// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgJobDao is the data access object for the table hg_youban_publish_tg_job.
type YoubanPublishTgJobDao struct {
	table    string                    // table is the underlying table name of the DAO.
	group    string                    // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgJobColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler        // handlers for customized model modification.
}

// YoubanPublishTgJobColumns defines and stores column names for the table hg_youban_publish_tg_job.
type YoubanPublishTgJobColumns struct {
	Id           string // 主键
	TaskId       string // 任务ID
	MerchantId   string // 商家ID
	AccountId    string // 账号ID
	ProfileId    string // 资料ID
	BotId        string // Bot ID
	TargetChatId string // 目标Chat ID
	TgMessageId  string // TG消息ID
	Status       string // 状态
	RetryCount   string // 重试次数
	NextRetryAt  string // 下次重试时间
	ErrorMessage string // 错误信息
	CreatedAt    string // 创建时间
	UpdatedAt    string // 更新时间
}

// youbanPublishTgJobColumns holds the columns for the table hg_youban_publish_tg_job.
var youbanPublishTgJobColumns = YoubanPublishTgJobColumns{
	Id:           "id",
	TaskId:       "task_id",
	MerchantId:   "merchant_id",
	AccountId:    "account_id",
	ProfileId:    "profile_id",
	BotId:        "bot_id",
	TargetChatId: "target_chat_id",
	TgMessageId:  "tg_message_id",
	Status:       "status",
	RetryCount:   "retry_count",
	NextRetryAt:  "next_retry_at",
	ErrorMessage: "error_message",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
}

// NewYoubanPublishTgJobDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgJobDao(handlers ...gdb.ModelHandler) *YoubanPublishTgJobDao {
	return &YoubanPublishTgJobDao{
		group:    "default",
		table:    "hg_youban_publish_tg_job",
		columns:  youbanPublishTgJobColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgJobDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgJobDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgJobDao) Columns() YoubanPublishTgJobColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgJobDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgJobDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgJobDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
