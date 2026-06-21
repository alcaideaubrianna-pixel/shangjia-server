// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// ContentMediaDao is the data access object for the table hg_content_media.
type ContentMediaDao struct {
	table    string              // table is the underlying table name of the DAO.
	group    string              // group is the database configuration group name of the current DAO.
	columns  ContentMediaColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler  // handlers for customized model modification.
}

// ContentMediaColumns defines and stores column names for the table hg_content_media.
type ContentMediaColumns struct {
	Id                  string // ID
	ProfileId           string // 资料ID
	SourceAssetId       string // FeiNiu资源ID
	DuplicateOfMediaId  string // 重复媒体ID
	MediaType           string // 媒体类型
	SortIndex           string // 排序
	OriginalStoragePath string // 原始存储路径
	DisplayStoragePath  string // 展示存储路径
	PreviewStoragePath  string // 预览存储路径
	BinaryMd5           string // 文件MD5
	PerceptualHash      string // 感知哈希
	Width               string // 宽度
	Height              string // 高度
	Duration            string // 时长
	ProcessStatus       string // 处理状态
	EncryptStatus       string // 加密状态
	Status              string // 状态
	CreatedAt           string // 创建时间
	UpdatedAt           string // 更新时间
	DeletedAt           string // 删除时间
}

// contentMediaColumns holds the columns for the table hg_content_media.
var contentMediaColumns = ContentMediaColumns{
	Id:                  "id",
	ProfileId:           "profile_id",
	SourceAssetId:       "source_asset_id",
	DuplicateOfMediaId:  "duplicate_of_media_id",
	MediaType:           "media_type",
	SortIndex:           "sort_index",
	OriginalStoragePath: "original_storage_path",
	DisplayStoragePath:  "display_storage_path",
	PreviewStoragePath:  "preview_storage_path",
	BinaryMd5:           "binary_md5",
	PerceptualHash:      "perceptual_hash",
	Width:               "width",
	Height:              "height",
	Duration:            "duration",
	ProcessStatus:       "process_status",
	EncryptStatus:       "encrypt_status",
	Status:              "status",
	CreatedAt:           "created_at",
	UpdatedAt:           "updated_at",
	DeletedAt:           "deleted_at",
}

// NewContentMediaDao creates and returns a new DAO object for table data access.
func NewContentMediaDao(handlers ...gdb.ModelHandler) *ContentMediaDao {
	return &ContentMediaDao{
		group:    "default",
		table:    "hg_content_media",
		columns:  contentMediaColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *ContentMediaDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *ContentMediaDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *ContentMediaDao) Columns() ContentMediaColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *ContentMediaDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *ContentMediaDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *ContentMediaDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
