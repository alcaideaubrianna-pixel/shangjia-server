// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTgLoginDao is the data access object for the table hg_youban_publish_tg_login.
type YoubanPublishTgLoginDao struct {
	table    string                      // table is the underlying table name of the DAO.
	group    string                      // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTgLoginColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler          // handlers for customized model modification.
}

// YoubanPublishTgLoginColumns defines and stores column names for the table hg_youban_publish_tg_login.
type YoubanPublishTgLoginColumns struct {
	Id               string //
	MerchantId       string //
	AccountId        string //
	LoginToken       string //
	QrUrl            string //
	TelegramUserId   string //
	TelegramUsername string //
	SessionKey       string //
	Status           string //
	ErrorMessage     string //
	ExpiresAt        string //
	CreatedAt        string //
	UpdatedAt        string //
	TenantId         string //
}

// youbanPublishTgLoginColumns holds the columns for the table hg_youban_publish_tg_login.
var youbanPublishTgLoginColumns = YoubanPublishTgLoginColumns{
	Id:               "id",
	MerchantId:       "merchant_id",
	AccountId:        "account_id",
	LoginToken:       "login_token",
	QrUrl:            "qr_url",
	TelegramUserId:   "telegram_user_id",
	TelegramUsername: "telegram_username",
	SessionKey:       "session_key",
	Status:           "status",
	ErrorMessage:     "error_message",
	ExpiresAt:        "expires_at",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
	TenantId:         "tenant_id",
}

// NewYoubanPublishTgLoginDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTgLoginDao(handlers ...gdb.ModelHandler) *YoubanPublishTgLoginDao {
	return &YoubanPublishTgLoginDao{
		group:    "default",
		table:    "hg_youban_publish_tg_login",
		columns:  youbanPublishTgLoginColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTgLoginDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTgLoginDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTgLoginDao) Columns() YoubanPublishTgLoginColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTgLoginDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTgLoginDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTgLoginDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
