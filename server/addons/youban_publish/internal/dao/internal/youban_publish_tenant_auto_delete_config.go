// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTenantAutoDeleteConfigDao is the data access object for the table hg_youban_publish_tenant_auto_delete_config.
type YoubanPublishTenantAutoDeleteConfigDao struct {
	table    string                                     // table is the underlying table name of the DAO.
	group    string                                     // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTenantAutoDeleteConfigColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                         // handlers for customized model modification.
}

// YoubanPublishTenantAutoDeleteConfigColumns defines and stores column names for the table hg_youban_publish_tenant_auto_delete_config.
type YoubanPublishTenantAutoDeleteConfigColumns struct {
	Id                 string //
	TenantId           string //
	Enabled            string //
	BotIdsJson         string //
	CustomKeywordsJson string //
	CustomRulesJson    string //
	CreatedBy          string //
	UpdatedBy          string //
	CreatedAt          string //
	UpdatedAt          string //
}

// youbanPublishTenantAutoDeleteConfigColumns holds the columns for the table hg_youban_publish_tenant_auto_delete_config.
var youbanPublishTenantAutoDeleteConfigColumns = YoubanPublishTenantAutoDeleteConfigColumns{
	Id:                 "id",
	TenantId:           "tenant_id",
	Enabled:            "enabled",
	BotIdsJson:         "bot_ids_json",
	CustomKeywordsJson: "custom_keywords_json",
	CustomRulesJson:    "custom_rules_json",
	CreatedBy:          "created_by",
	UpdatedBy:          "updated_by",
	CreatedAt:          "created_at",
	UpdatedAt:          "updated_at",
}

// NewYoubanPublishTenantAutoDeleteConfigDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTenantAutoDeleteConfigDao(handlers ...gdb.ModelHandler) *YoubanPublishTenantAutoDeleteConfigDao {
	return &YoubanPublishTenantAutoDeleteConfigDao{
		group:    "default",
		table:    "hg_youban_publish_tenant_auto_delete_config",
		columns:  youbanPublishTenantAutoDeleteConfigColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTenantAutoDeleteConfigDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTenantAutoDeleteConfigDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTenantAutoDeleteConfigDao) Columns() YoubanPublishTenantAutoDeleteConfigColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTenantAutoDeleteConfigDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTenantAutoDeleteConfigDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTenantAutoDeleteConfigDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
