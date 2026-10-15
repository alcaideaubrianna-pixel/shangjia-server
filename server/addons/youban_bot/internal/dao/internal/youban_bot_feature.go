// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotFeatureDao is the data access object for the table hg_youban_bot_feature.
type YoubanBotFeatureDao struct {
	table    string                  // table is the underlying table name of the DAO.
	group    string                  // group is the database configuration group name of the current DAO.
	columns  YoubanBotFeatureColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler      // handlers for customized model modification.
}

// YoubanBotFeatureColumns defines and stores column names for the table hg_youban_bot_feature.
type YoubanBotFeatureColumns struct {
	Id          string //
	FeatureKey  string //
	Name        string //
	Command     string //
	Description string //
	ConfigJson  string //
	Sort        string //
	Status      string //
	CreatedAt   string //
	UpdatedAt   string //
	DeletedAt   string //
}

// youbanBotFeatureColumns holds the columns for the table hg_youban_bot_feature.
var youbanBotFeatureColumns = YoubanBotFeatureColumns{
	Id:          "id",
	FeatureKey:  "feature_key",
	Name:        "name",
	Command:     "command",
	Description: "description",
	ConfigJson:  "config_json",
	Sort:        "sort",
	Status:      "status",
	CreatedAt:   "created_at",
	UpdatedAt:   "updated_at",
	DeletedAt:   "deleted_at",
}

// NewYoubanBotFeatureDao creates and returns a new DAO object for table data access.
func NewYoubanBotFeatureDao(handlers ...gdb.ModelHandler) *YoubanBotFeatureDao {
	return &YoubanBotFeatureDao{
		group:    "default",
		table:    "hg_youban_bot_feature",
		columns:  youbanBotFeatureColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotFeatureDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotFeatureDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotFeatureDao) Columns() YoubanBotFeatureColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotFeatureDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotFeatureDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotFeatureDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
