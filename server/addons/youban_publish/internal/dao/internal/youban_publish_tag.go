// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTagDao is the data access object for the table hg_youban_publish_tag.
type YoubanPublishTagDao struct {
	table    string                  // table is the underlying table name of the DAO.
	group    string                  // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTagColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler      // handlers for customized model modification.
}

// YoubanPublishTagColumns defines and stores column names for the table hg_youban_publish_tag.
type YoubanPublishTagColumns struct {
	Id           string // 主键
	Name         string // 标签名称
	ReviewStatus string // 审核状态
	Status       string // 状态
	UseCount     string // 使用数量
	CreatedBy    string // 创建人
	UpdatedBy    string // 更新人
	DeletedBy    string // 删除人
	CreatedAt    string // 创建时间
	UpdatedAt    string // 更新时间
	DeletedAt    string // 删除时间
}

// youbanPublishTagColumns holds the columns for the table hg_youban_publish_tag.
var youbanPublishTagColumns = YoubanPublishTagColumns{
	Id:           "id",
	Name:         "name",
	ReviewStatus: "review_status",
	Status:       "status",
	UseCount:     "use_count",
	CreatedBy:    "created_by",
	UpdatedBy:    "updated_by",
	DeletedBy:    "deleted_by",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
	DeletedAt:    "deleted_at",
}

// NewYoubanPublishTagDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTagDao(handlers ...gdb.ModelHandler) *YoubanPublishTagDao {
	return &YoubanPublishTagDao{
		group:    "default",
		table:    "hg_youban_publish_tag",
		columns:  youbanPublishTagColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTagDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTagDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTagDao) Columns() YoubanPublishTagColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTagDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTagDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTagDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
