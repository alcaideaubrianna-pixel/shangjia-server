// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgMessageDao is the data access object for the table hg_youban_publish_tg_message.
type YoubanPublishTgMessageDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgMessageColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanPublishTgMessageColumns defines and stores column names for the table hg_youban_publish_tg_message.
type YoubanPublishTgMessageColumns struct {
	Id           string // 主键
	JobId        string // TG任务ID
	TaskId       string // 任务ID
	TenantId     string // 租户ID
	AccountId    string // 账号ID
	ProfileId    string // 资料ID
	BotId        string // Bot ID
	TargetChatId string // 目标Chat ID
	TgMessageId  string // TG消息ID
	MediaGroupId string // 媒体组ID
	MediaId      string // 媒体ID
	Purpose      string // display/verify
	TgFileId     string // TG文件ID
	Status       string // 状态
	SentAt       string // 发送时间
	DeletedAt    string // 删除时间
	CreatedAt    string // 创建时间
	UpdatedAt    string // 更新时间
}

// youbanPublishTgMessageColumns holds the columns for the table hg_youban_publish_tg_message.
var youbanPublishTgMessageColumns = YoubanPublishTgMessageColumns{
	Id:           "id",
	JobId:        "job_id",
	TaskId:       "task_id",
	TenantId:     "tenant_id",
	AccountId:    "account_id",
	ProfileId:    "profile_id",
	BotId:        "bot_id",
	TargetChatId: "target_chat_id",
	TgMessageId:  "tg_message_id",
	MediaGroupId: "media_group_id",
	MediaId:      "media_id",
	Purpose:      "purpose",
	TgFileId:     "tg_file_id",
	Status:       "status",
	SentAt:       "sent_at",
	DeletedAt:    "deleted_at",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
}

// NewYoubanPublishTgMessageDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgMessageDao(handlers ...gdb.ModelHandler) *YoubanPublishTgMessageDao {
	return &YoubanPublishTgMessageDao{
		group:    "default",
		table:    "hg_youban_publish_tg_message",
		columns:  youbanPublishTgMessageColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgMessageDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgMessageDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgMessageDao) Columns() YoubanPublishTgMessageColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgMessageDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgMessageDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgMessageDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
