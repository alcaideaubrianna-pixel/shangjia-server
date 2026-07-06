// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishImportTaskDao is the data access object for the table hg_youban_publish_import_task.
type YoubanPublishImportTaskDao struct {
	table    string                         // table is the underlying table name of the DAO.
	group    string                         // group is the database configuration group name of the current DAO.
	columns  YoubanPublishImportTaskColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler             // handlers for customized model modification.
}

// YoubanPublishImportTaskColumns defines and stores column names for the table hg_youban_publish_import_task.
type YoubanPublishImportTaskColumns struct {
	Id               string // 主键
	TenantId         string // 租户ID
	AccountId        string // 上架账号ID
	SourceName       string // 来源名称
	BaseUrl          string // 旧站域名
	Username         string // 旧站账号
	PasswordCipher   string // 旧站密码密文
	LimitCount       string // 测试采集数量
	PerPage          string // 每页数量
	ProxyEnabled     string // 是否启用代理
	ProxyPool        string // 代理池
	MediaConcurrency string // 媒体并发数
	ChannelIdJson    string // 匹配频道ID JSON
	TgStartAt        string // TG匹配开始时间
	TgEndAt          string // TG匹配结束时间
	Status           string // 任务状态
	Stage            string // 执行阶段
	ProgressTotal    string // 总进度
	ProgressDone     string // 已完成进度
	PageTotal        string // 总页数
	PageDone         string // 已完成页数
	ItemTotal        string // 资料总数
	ItemDone         string // 已处理资料数
	Imported         string // 导入数量
	Duplicate        string // 重复数量
	MediaTotal       string // 媒体总数
	MediaDone        string // 已处理媒体数
	MediaImported    string // 媒体导入数量
	TgTotal          string // TG消息总数
	TgDone           string // TG已处理数
	TgMatched        string // TG匹配数量
	LastSourceNoteId string // 最近旧站资料ID
	ErrorMessage     string // 错误信息
	ResultJson       string // 执行结果JSON
	Remark           string // 备注
	CreatedBy        string // 创建人
	UpdatedBy        string // 更新人
	DeletedBy        string // 删除人
	StartedAt        string // 开始时间
	FinishedAt       string // 结束时间
	CreatedAt        string // 创建时间
	UpdatedAt        string // 更新时间
	DeletedAt        string // 删除时间
}

// youbanPublishImportTaskColumns holds the columns for the table hg_youban_publish_import_task.
var youbanPublishImportTaskColumns = YoubanPublishImportTaskColumns{
	Id:               "id",
	TenantId:         "tenant_id",
	AccountId:        "account_id",
	SourceName:       "source_name",
	BaseUrl:          "base_url",
	Username:         "username",
	PasswordCipher:   "password_cipher",
	LimitCount:       "limit_count",
	PerPage:          "per_page",
	ProxyEnabled:     "proxy_enabled",
	ProxyPool:        "proxy_pool",
	MediaConcurrency: "media_concurrency",
	ChannelIdJson:    "channel_id_json",
	TgStartAt:        "tg_start_at",
	TgEndAt:          "tg_end_at",
	Status:           "status",
	Stage:            "stage",
	ProgressTotal:    "progress_total",
	ProgressDone:     "progress_done",
	PageTotal:        "page_total",
	PageDone:         "page_done",
	ItemTotal:        "item_total",
	ItemDone:         "item_done",
	Imported:         "imported",
	Duplicate:        "duplicate",
	MediaTotal:       "media_total",
	MediaDone:        "media_done",
	MediaImported:    "media_imported",
	TgTotal:          "tg_total",
	TgDone:           "tg_done",
	TgMatched:        "tg_matched",
	LastSourceNoteId: "last_source_note_id",
	ErrorMessage:     "error_message",
	ResultJson:       "result_json",
	Remark:           "remark",
	CreatedBy:        "created_by",
	UpdatedBy:        "updated_by",
	DeletedBy:        "deleted_by",
	StartedAt:        "started_at",
	FinishedAt:       "finished_at",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
	DeletedAt:        "deleted_at",
}

// NewYoubanPublishImportTaskDao creates and returns a new DAO object for table data access.
func NewYoubanPublishImportTaskDao(handlers ...gdb.ModelHandler) *YoubanPublishImportTaskDao {
	return &YoubanPublishImportTaskDao{
		group:    "default",
		table:    "hg_youban_publish_import_task",
		columns:  youbanPublishImportTaskColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishImportTaskDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishImportTaskDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishImportTaskDao) Columns() YoubanPublishImportTaskColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishImportTaskDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishImportTaskDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishImportTaskDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
