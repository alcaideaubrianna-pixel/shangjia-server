// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanTwoWayBotCooperationBlacklistDao is the data access object for the table hg_youban_two_way_bot_cooperation_blacklist.
type YoubanTwoWayBotCooperationBlacklistDao struct {
	table    string                                     // table is the underlying table name of the DAO.
	group    string                                     // group is the database configuration group name of the current DAO.
	columns  YoubanTwoWayBotCooperationBlacklistColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                         // handlers for customized model modification.
}

// YoubanTwoWayBotCooperationBlacklistColumns defines and stores column names for the table hg_youban_two_way_bot_cooperation_blacklist.
type YoubanTwoWayBotCooperationBlacklistColumns struct {
	Id                 string //
	TenantId           string //
	ConfigId           string //
	ApplicantTgUserId  string //
	ApplicantUsername  string //
	ApplicantFirstName string //
	ApplicantLastName  string //
	Reason             string //
	Status             string //
	CreatedBy          string //
	UpdatedBy          string //
	CreatedAt          string //
	UpdatedAt          string //
}

// youbanTwoWayBotCooperationBlacklistColumns holds the columns for the table hg_youban_two_way_bot_cooperation_blacklist.
var youbanTwoWayBotCooperationBlacklistColumns = YoubanTwoWayBotCooperationBlacklistColumns{
	Id:                 "id",
	TenantId:           "tenant_id",
	ConfigId:           "config_id",
	ApplicantTgUserId:  "applicant_tg_user_id",
	ApplicantUsername:  "applicant_username",
	ApplicantFirstName: "applicant_first_name",
	ApplicantLastName:  "applicant_last_name",
	Reason:             "reason",
	Status:             "status",
	CreatedBy:          "created_by",
	UpdatedBy:          "updated_by",
	CreatedAt:          "created_at",
	UpdatedAt:          "updated_at",
}

// NewYoubanTwoWayBotCooperationBlacklistDao creates and returns a new DAO object for table data access.
func NewYoubanTwoWayBotCooperationBlacklistDao(handlers ...gdb.ModelHandler) *YoubanTwoWayBotCooperationBlacklistDao {
	return &YoubanTwoWayBotCooperationBlacklistDao{
		group:    "default",
		table:    "hg_youban_two_way_bot_cooperation_blacklist",
		columns:  youbanTwoWayBotCooperationBlacklistColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanTwoWayBotCooperationBlacklistDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanTwoWayBotCooperationBlacklistDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanTwoWayBotCooperationBlacklistDao) Columns() YoubanTwoWayBotCooperationBlacklistColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanTwoWayBotCooperationBlacklistDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanTwoWayBotCooperationBlacklistDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanTwoWayBotCooperationBlacklistDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
