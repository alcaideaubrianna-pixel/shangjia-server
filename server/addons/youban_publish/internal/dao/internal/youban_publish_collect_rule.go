// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectRuleDao is the data access object for the table hg_youban_publish_collect_rule.
type YoubanPublishCollectRuleDao struct {
	table    string                          // table is the underlying table name of the DAO.
	group    string                          // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectRuleColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler              // handlers for customized model modification.
}

// YoubanPublishCollectRuleColumns defines and stores column names for the table hg_youban_publish_collect_rule.
type YoubanPublishCollectRuleColumns struct {
	Id                   string // 主键
	TenantId             string // 租户ID
	AccountId            string // 所属账号ID
	Name                 string // 规则名称
	GlobalEnabled        string // 是否全局应用
	TargetChannelIdJson  string // 目标频道ID JSON
	BotIdJson            string // 推送BOT ID JSON
	BackupChannelId      string // 备份群ID
	ReviewEnabled        string // 是否需要审核
	DedupeEnabled        string // 是否图文去重
	DedupeDays           string // 去重天数
	KeywordJson          string // 关键词JSON
	TagJson              string // 标签JSON
	ReplaceJson          string // 替换规则JSON
	BlockTextJson        string // 屏蔽文本JSON
	BlockLink            string // 屏蔽链接
	BlockUsername        string // 屏蔽用户名
	BlockPlainText       string // 屏蔽纯文本
	MinMediaCountEnabled string // 是否限制媒体数量
	MinMediaCount        string // 最少媒体数
	HeaderEnabled        string // 是否启用前置文案
	HeaderMarkdown       string // 前置Markdown文案
	FooterEnabled        string // 是否启用后置文案
	FooterMarkdown       string // 后置Markdown文案
	Sort                 string // 排序
	Status               string // 状态
	CreatedBy            string // 创建人
	UpdatedBy            string // 更新人
	DeletedBy            string // 删除人
	CreatedAt            string // 创建时间
	UpdatedAt            string // 更新时间
	DeletedAt            string // 删除时间
}

// youbanPublishCollectRuleColumns holds the columns for the table hg_youban_publish_collect_rule.
var youbanPublishCollectRuleColumns = YoubanPublishCollectRuleColumns{
	Id:                   "id",
	TenantId:             "tenant_id",
	AccountId:            "account_id",
	Name:                 "name",
	GlobalEnabled:        "global_enabled",
	TargetChannelIdJson:  "target_channel_id_json",
	BotIdJson:            "bot_id_json",
	BackupChannelId:      "backup_channel_id",
	ReviewEnabled:        "review_enabled",
	DedupeEnabled:        "dedupe_enabled",
	DedupeDays:           "dedupe_days",
	KeywordJson:          "keyword_json",
	TagJson:              "tag_json",
	ReplaceJson:          "replace_json",
	BlockTextJson:        "block_text_json",
	BlockLink:            "block_link",
	BlockUsername:        "block_username",
	BlockPlainText:       "block_plain_text",
	MinMediaCountEnabled: "min_media_count_enabled",
	MinMediaCount:        "min_media_count",
	HeaderEnabled:        "header_enabled",
	HeaderMarkdown:       "header_markdown",
	FooterEnabled:        "footer_enabled",
	FooterMarkdown:       "footer_markdown",
	Sort:                 "sort",
	Status:               "status",
	CreatedBy:            "created_by",
	UpdatedBy:            "updated_by",
	DeletedBy:            "deleted_by",
	CreatedAt:            "created_at",
	UpdatedAt:            "updated_at",
	DeletedAt:            "deleted_at",
}

// NewYoubanPublishCollectRuleDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectRuleDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectRuleDao {
	return &YoubanPublishCollectRuleDao{
		group:    "default",
		table:    "hg_youban_publish_collect_rule",
		columns:  youbanPublishCollectRuleColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectRuleDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectRuleDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectRuleDao) Columns() YoubanPublishCollectRuleColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectRuleDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectRuleDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectRuleDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
