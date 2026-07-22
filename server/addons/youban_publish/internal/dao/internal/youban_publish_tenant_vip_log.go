// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTenantVipLogDao is the data access object for the table hg_youban_publish_tenant_vip_log.
type YoubanPublishTenantVipLogDao struct {
	table    string                           // table is the underlying table name of the DAO.
	group    string                           // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTenantVipLogColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler               // handlers for customized model modification.
}

// YoubanPublishTenantVipLogColumns defines and stores column names for the table hg_youban_publish_tenant_vip_log.
type YoubanPublishTenantVipLogColumns struct {
	Id              string //
	TenantId        string //
	OperatorId      string //
	Source          string //
	Action          string //
	BeforeStatus    string //
	BeforeLevel     string //
	BeforeExpiredAt string //
	AfterStatus     string //
	AfterLevel      string //
	AfterExpiredAt  string //
	Remark          string //
	CreatedAt       string //
}

// youbanPublishTenantVipLogColumns holds the columns for the table hg_youban_publish_tenant_vip_log.
var youbanPublishTenantVipLogColumns = YoubanPublishTenantVipLogColumns{
	Id:              "id",
	TenantId:        "tenant_id",
	OperatorId:      "operator_id",
	Source:          "source",
	Action:          "action",
	BeforeStatus:    "before_status",
	BeforeLevel:     "before_level",
	BeforeExpiredAt: "before_expired_at",
	AfterStatus:     "after_status",
	AfterLevel:      "after_level",
	AfterExpiredAt:  "after_expired_at",
	Remark:          "remark",
	CreatedAt:       "created_at",
}

// NewYoubanPublishTenantVipLogDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTenantVipLogDao(handlers ...gdb.ModelHandler) *YoubanPublishTenantVipLogDao {
	return &YoubanPublishTenantVipLogDao{
		group:    "default",
		table:    "hg_youban_publish_tenant_vip_log",
		columns:  youbanPublishTenantVipLogColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTenantVipLogDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTenantVipLogDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTenantVipLogDao) Columns() YoubanPublishTenantVipLogColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTenantVipLogDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTenantVipLogDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTenantVipLogDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
