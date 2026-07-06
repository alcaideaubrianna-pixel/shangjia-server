// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectDispatchDao is the data access object for the table hg_youban_publish_collect_dispatch.
type YoubanPublishCollectDispatchDao struct {
	table    string                              // table is the underlying table name of the DAO.
	group    string                              // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectDispatchColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                  // handlers for customized model modification.
}

// YoubanPublishCollectDispatchColumns defines and stores column names for the table hg_youban_publish_collect_dispatch.
type YoubanPublishCollectDispatchColumns struct {
	Id                  string // 主键
	TenantId            string // 租户ID
	AccountId           string // 所属账号ID
	SourceId            string // 采集源ID
	RuleId              string // 规则ID
	EventId             string // 采集事件ID
	ReviewId            string // 审核ID
	ProfileId           string // 资料ID
	TaskId              string // 上架任务ID
	TargetChannelIdJson string // 目标频道ID JSON
	BotIdJson           string // 推送BOT ID JSON
	MatchJson           string // 命中详情JSON
	Status              string // 状态
	ErrorMessage        string // 错误信息
	CreatedAt           string // 创建时间
	UpdatedAt           string // 更新时间
	FinishedAt          string // 完成时间
}

// youbanPublishCollectDispatchColumns holds the columns for the table hg_youban_publish_collect_dispatch.
var youbanPublishCollectDispatchColumns = YoubanPublishCollectDispatchColumns{
	Id:                  "id",
	TenantId:            "tenant_id",
	AccountId:           "account_id",
	SourceId:            "source_id",
	RuleId:              "rule_id",
	EventId:             "event_id",
	ReviewId:            "review_id",
	ProfileId:           "profile_id",
	TaskId:              "task_id",
	TargetChannelIdJson: "target_channel_id_json",
	BotIdJson:           "bot_id_json",
	MatchJson:           "match_json",
	Status:              "status",
	ErrorMessage:        "error_message",
	CreatedAt:           "created_at",
	UpdatedAt:           "updated_at",
	FinishedAt:          "finished_at",
}

// NewYoubanPublishCollectDispatchDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectDispatchDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectDispatchDao {
	return &YoubanPublishCollectDispatchDao{
		group:    "default",
		table:    "hg_youban_publish_collect_dispatch",
		columns:  youbanPublishCollectDispatchColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectDispatchDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectDispatchDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectDispatchDao) Columns() YoubanPublishCollectDispatchColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectDispatchDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectDispatchDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectDispatchDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
