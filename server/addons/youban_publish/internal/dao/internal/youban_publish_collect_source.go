// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectSourceDao is the data access object for the table hg_youban_publish_collect_source.
type YoubanPublishCollectSourceDao struct {
	table    string                            // table is the underlying table name of the DAO.
	group    string                            // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectSourceColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                // handlers for customized model modification.
}

// YoubanPublishCollectSourceColumns defines and stores column names for the table hg_youban_publish_collect_source.
type YoubanPublishCollectSourceColumns struct {
	Id              string // 主键
	TenantId        string // 租户ID
	AccountId       string // 所属账号ID
	SourceType      string // 来源类型
	Title           string // 采集源名称
	SourceChatId    string // 来源频道/群聊ID
	SourceUsername  string // 来源用户名
	TgAccountId     string // 协议号ID
	BotId           string // 机器人ID
	FollowAccountId string // 关注账号ID
	CollectEnabled  string // 是否开启采集
	Status          string // 状态
	EventTotal      string // 事件总数
	SuccessTotal    string // 成功数
	FailedTotal     string // 失败数
	LastEventAt     string // 最后事件时间
	Remark          string // 备注
	CreatedBy       string // 创建人
	UpdatedBy       string // 更新人
	DeletedBy       string // 删除人
	CreatedAt       string // 创建时间
	UpdatedAt       string // 更新时间
	DeletedAt       string // 删除时间
}

// youbanPublishCollectSourceColumns holds the columns for the table hg_youban_publish_collect_source.
var youbanPublishCollectSourceColumns = YoubanPublishCollectSourceColumns{
	Id:              "id",
	TenantId:        "tenant_id",
	AccountId:       "account_id",
	SourceType:      "source_type",
	Title:           "title",
	SourceChatId:    "source_chat_id",
	SourceUsername:  "source_username",
	TgAccountId:     "tg_account_id",
	BotId:           "bot_id",
	FollowAccountId: "follow_account_id",
	CollectEnabled:  "collect_enabled",
	Status:          "status",
	EventTotal:      "event_total",
	SuccessTotal:    "success_total",
	FailedTotal:     "failed_total",
	LastEventAt:     "last_event_at",
	Remark:          "remark",
	CreatedBy:       "created_by",
	UpdatedBy:       "updated_by",
	DeletedBy:       "deleted_by",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
	DeletedAt:       "deleted_at",
}

// NewYoubanPublishCollectSourceDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectSourceDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectSourceDao {
	return &YoubanPublishCollectSourceDao{
		group:    "default",
		table:    "hg_youban_publish_collect_source",
		columns:  youbanPublishCollectSourceColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectSourceDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectSourceDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectSourceDao) Columns() YoubanPublishCollectSourceColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectSourceDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectSourceDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectSourceDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
