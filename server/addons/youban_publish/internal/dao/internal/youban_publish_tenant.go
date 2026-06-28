// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTenantDao is the data access object for the table hg_youban_publish_tenant.
type YoubanPublishTenantDao struct {
	table    string                     // table is the underlying table name of the DAO.
	group    string                     // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTenantColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler         // handlers for customized model modification.
}

// YoubanPublishTenantColumns defines and stores column names for the table hg_youban_publish_tenant.
type YoubanPublishTenantColumns struct {
	Id           string //
	Name         string //
	ContactName  string //
	ContactPhone string //
	Remark       string //
	Status       string //
	CreatedBy    string //
	UpdatedBy    string //
	DeletedBy    string //
	CreatedAt    string //
	UpdatedAt    string //
	DeletedAt    string //
}

// youbanPublishTenantColumns holds the columns for the table hg_youban_publish_tenant.
var youbanPublishTenantColumns = YoubanPublishTenantColumns{
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

// NewYoubanPublishTenantDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTenantDao(handlers ...gdb.ModelHandler) *YoubanPublishTenantDao {
	return &YoubanPublishTenantDao{
		group:    "default",
		table:    "hg_youban_publish_tenant",
		columns:  youbanPublishTenantColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTenantDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTenantDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTenantDao) Columns() YoubanPublishTenantColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTenantDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTenantDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTenantDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
