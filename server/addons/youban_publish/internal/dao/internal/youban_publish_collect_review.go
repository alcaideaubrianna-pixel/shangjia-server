// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectReviewDao is the data access object for the table hg_youban_publish_collect_review.
type YoubanPublishCollectReviewDao struct {
	table    string                            // table is the underlying table name of the DAO.
	group    string                            // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectReviewColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                // handlers for customized model modification.
}

// YoubanPublishCollectReviewColumns defines and stores column names for the table hg_youban_publish_collect_review.
type YoubanPublishCollectReviewColumns struct {
	Id                  string // 主键
	TenantId            string // 租户ID
	AccountId           string // 所属账号ID
	SourceId            string // 采集源ID
	RuleId              string // 规则ID
	EventId             string // 采集事件ID
	DispatchId          string // 分发记录ID
	RawText             string // 原始文本
	MediaCount          string // 媒体数量
	TargetChannelIdJson string // 目标频道ID JSON
	BotIdJson           string // 推送BOT ID JSON
	Status              string // 审核状态
	ReviewReason        string // 审核原因
	ReviewedBy          string // 审核人
	ReviewedAt          string // 审核时间
	CreatedAt           string // 创建时间
	UpdatedAt           string // 更新时间
}

// youbanPublishCollectReviewColumns holds the columns for the table hg_youban_publish_collect_review.
var youbanPublishCollectReviewColumns = YoubanPublishCollectReviewColumns{
	Id:                  "id",
	TenantId:            "tenant_id",
	AccountId:           "account_id",
	SourceId:            "source_id",
	RuleId:              "rule_id",
	EventId:             "event_id",
	DispatchId:          "dispatch_id",
	RawText:             "raw_text",
	MediaCount:          "media_count",
	TargetChannelIdJson: "target_channel_id_json",
	BotIdJson:           "bot_id_json",
	Status:              "status",
	ReviewReason:        "review_reason",
	ReviewedBy:          "reviewed_by",
	ReviewedAt:          "reviewed_at",
	CreatedAt:           "created_at",
	UpdatedAt:           "updated_at",
}

// NewYoubanPublishCollectReviewDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectReviewDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectReviewDao {
	return &YoubanPublishCollectReviewDao{
		group:    "default",
		table:    "hg_youban_publish_collect_review",
		columns:  youbanPublishCollectReviewColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectReviewDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectReviewDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectReviewDao) Columns() YoubanPublishCollectReviewColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectReviewDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectReviewDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectReviewDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
