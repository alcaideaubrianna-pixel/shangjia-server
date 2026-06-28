// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishAccountDao is the data access object for the table hg_youban_publish_account.
type YoubanPublishAccountDao struct {
	table    string                      // table is the underlying table name of the DAO.
	group    string                      // group is the database configuration group name of the current DAO.
	columns  YoubanPublishAccountColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler          // handlers for customized model modification.
}

// YoubanPublishAccountColumns defines and stores column names for the table hg_youban_publish_account.
type YoubanPublishAccountColumns struct {
	Id                 string //
	MerchantId         string //
	AdminMemberId      string //
	ParentId           string //
	AccountType        string //
	Nickname           string //
	Username           string //
	TelegramUserId     string //
	TelegramUsername   string //
	DailyPublishLimit  string //
	CanDirectPublish   string //
	AllowedChannelJson string //
	AllowedRegionJson  string //
	Remark             string //
	Status             string //
	CreatedBy          string //
	UpdatedBy          string //
	DeletedBy          string //
	CreatedAt          string //
	UpdatedAt          string //
	DeletedAt          string //
	TenantId           string //
}

// youbanPublishAccountColumns holds the columns for the table hg_youban_publish_account.
var youbanPublishAccountColumns = YoubanPublishAccountColumns{
	Id:                 "id",
	MerchantId:         "merchant_id",
	AdminMemberId:      "admin_member_id",
	ParentId:           "parent_id",
	AccountType:        "account_type",
	Nickname:           "nickname",
	Username:           "username",
	TelegramUserId:     "telegram_user_id",
	TelegramUsername:   "telegram_username",
	DailyPublishLimit:  "daily_publish_limit",
	CanDirectPublish:   "can_direct_publish",
	AllowedChannelJson: "allowed_channel_json",
	AllowedRegionJson:  "allowed_region_json",
	Remark:             "remark",
	Status:             "status",
	CreatedBy:          "created_by",
	UpdatedBy:          "updated_by",
	DeletedBy:          "deleted_by",
	CreatedAt:          "created_at",
	UpdatedAt:          "updated_at",
	DeletedAt:          "deleted_at",
	TenantId:           "tenant_id",
}

// NewYoubanPublishAccountDao creates and returns a new DAO object for table data access.
func NewYoubanPublishAccountDao(handlers ...gdb.ModelHandler) *YoubanPublishAccountDao {
	return &YoubanPublishAccountDao{
		group:    "default",
		table:    "hg_youban_publish_account",
		columns:  youbanPublishAccountColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishAccountDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishAccountDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishAccountDao) Columns() YoubanPublishAccountColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishAccountDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishAccountDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishAccountDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
