// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishMediaDao is the data access object for the table hg_youban_publish_media.
type YoubanPublishMediaDao struct {
	table    string                    // table is the underlying table name of the DAO.
	group    string                    // group is the database configuration group name of the current DAO.
	columns  YoubanPublishMediaColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler        // handlers for customized model modification.
}

// YoubanPublishMediaColumns defines and stores column names for the table hg_youban_publish_media.
type YoubanPublishMediaColumns struct {
	Id           string // 主键
	MerchantId   string // 商家ID
	AccountId    string // 账号ID
	TaskId       string // 任务ID
	ProfileId    string // 资料ID
	AttachmentId string // HotGo附件ID
	MediaType    string // 媒体类型
	Name         string // 文件名
	FileUrl      string // 访问地址
	StoragePath  string // 存储路径
	MimeType     string // MIME
	Md5          string // MD5
	Size         string // 大小
	SortIndex    string // 排序
	Status       string // 状态
	CreatedBy    string // 创建人
	UpdatedBy    string // 更新人
	DeletedBy    string // 删除人
	CreatedAt    string // 创建时间
	UpdatedAt    string // 更新时间
	DeletedAt    string // 删除时间
}

// youbanPublishMediaColumns holds the columns for the table hg_youban_publish_media.
var youbanPublishMediaColumns = YoubanPublishMediaColumns{
	Id:           "id",
	MerchantId:   "merchant_id",
	AccountId:    "account_id",
	TaskId:       "task_id",
	ProfileId:    "profile_id",
	AttachmentId: "attachment_id",
	MediaType:    "media_type",
	Name:         "name",
	FileUrl:      "file_url",
	StoragePath:  "storage_path",
	MimeType:     "mime_type",
	Md5:          "md5",
	Size:         "size",
	SortIndex:    "sort_index",
	Status:       "status",
	CreatedBy:    "created_by",
	UpdatedBy:    "updated_by",
	DeletedBy:    "deleted_by",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
	DeletedAt:    "deleted_at",
}

// NewYoubanPublishMediaDao creates and returns a new DAO object for table data access.
func NewYoubanPublishMediaDao(handlers ...gdb.ModelHandler) *YoubanPublishMediaDao {
	return &YoubanPublishMediaDao{
		group:    "default",
		table:    "hg_youban_publish_media",
		columns:  youbanPublishMediaColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishMediaDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishMediaDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishMediaDao) Columns() YoubanPublishMediaColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishMediaDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishMediaDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishMediaDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
