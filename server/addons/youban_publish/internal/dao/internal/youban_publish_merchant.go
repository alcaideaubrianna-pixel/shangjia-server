// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishMerchantDao is the data access object for the table hg_youban_publish_merchant.
type YoubanPublishMerchantDao struct {
	table    string                       // table is the underlying table name of the DAO.
	group    string                       // group is the database configuration group name of the current DAO.
	columns  YoubanPublishMerchantColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler           // handlers for customized model modification.
}

// YoubanPublishMerchantColumns defines and stores column names for the table hg_youban_publish_merchant.
type YoubanPublishMerchantColumns struct {
	Id           string // 主键
	Name         string // 商家名称
	ContactName  string // 联系人
	ContactPhone string // 联系电话
	Remark       string // 备注
	Status       string // 状态
	CreatedBy    string // 创建人
	UpdatedBy    string // 更新人
	DeletedBy    string // 删除人
	CreatedAt    string // 创建时间
	UpdatedAt    string // 更新时间
	DeletedAt    string // 删除时间
}

// youbanPublishMerchantColumns holds the columns for the table hg_youban_publish_merchant.
var youbanPublishMerchantColumns = YoubanPublishMerchantColumns{
	Id:           "id",
	Name:         "name",
	ContactName:  "contact_name",
	ContactPhone: "contact_phone",
	Remark:       "remark",
	Status:       "status",
	CreatedBy:    "created_by",
	UpdatedBy:    "updated_by",
	DeletedBy:    "deleted_by",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
	DeletedAt:    "deleted_at",
}

// NewYoubanPublishMerchantDao creates and returns a new DAO object for table data access.
func NewYoubanPublishMerchantDao(handlers ...gdb.ModelHandler) *YoubanPublishMerchantDao {
	return &YoubanPublishMerchantDao{
		group:    "default",
		table:    "hg_youban_publish_merchant",
		columns:  youbanPublishMerchantColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishMerchantDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishMerchantDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishMerchantDao) Columns() YoubanPublishMerchantColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishMerchantDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishMerchantDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishMerchantDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
