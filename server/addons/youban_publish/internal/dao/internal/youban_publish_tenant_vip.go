// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTenantVipDao is the data access object for the table hg_youban_publish_tenant_vip.
type YoubanPublishTenantVipDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTenantVipColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanPublishTenantVipColumns defines and stores column names for the table hg_youban_publish_tenant_vip.
type YoubanPublishTenantVipColumns struct {
	Id        string //
	TenantId  string //
	Level     string //
	Status    string //
	OpenedAt  string //
	ExpiredAt string //
	Remark    string //
	CreatedAt string //
	UpdatedAt string //
	DeletedAt string //
}

// youbanPublishTenantVipColumns holds the columns for the table hg_youban_publish_tenant_vip.
var youbanPublishTenantVipColumns = YoubanPublishTenantVipColumns{
	Id:        "id",
	TenantId:  "tenant_id",
	Level:     "level",
	Status:    "status",
	OpenedAt:  "opened_at",
	ExpiredAt: "expired_at",
	Remark:    "remark",
	CreatedAt: "created_at",
	UpdatedAt: "updated_at",
	DeletedAt: "deleted_at",
}

// NewYoubanPublishTenantVipDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTenantVipDao(handlers ...gdb.ModelHandler) *YoubanPublishTenantVipDao {
	return &YoubanPublishTenantVipDao{
		group:    "default",
		table:    "hg_youban_publish_tenant_vip",
		columns:  youbanPublishTenantVipColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTenantVipDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTenantVipDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTenantVipDao) Columns() YoubanPublishTenantVipColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTenantVipDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTenantVipDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTenantVipDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
