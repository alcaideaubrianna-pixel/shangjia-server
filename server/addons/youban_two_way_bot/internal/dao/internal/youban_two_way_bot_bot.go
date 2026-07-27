// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanTwoWayBotBotDao is the data access object for the table hg_youban_two_way_bot_bot.
type YoubanTwoWayBotBotDao struct {
	table    string                    // table is the underlying table name of the DAO.
	group    string                    // group is the database configuration group name of the current DAO.
	columns  YoubanTwoWayBotBotColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler        // handlers for customized model modification.
}

// YoubanTwoWayBotBotColumns defines and stores column names for the table hg_youban_two_way_bot_bot.
type YoubanTwoWayBotBotColumns struct {
	Id                   string //
	TenantId             string //
	AccountId            string //
	TgAccountId          string //
	Name                 string //
	BotToken             string //
	BotUserId            string //
	BotUsername          string //
	SupergroupId         string //
	SupergroupAccessHash string //
	SupergroupTitle      string //
	InviteLink           string //
	SetupStatus          string //
	WebhookStatus        string //
	Status               string //
	ErrorMessage         string //
	LastSetupAt          string //
	LastWebhookAt        string //
	CreatedAt            string //
	UpdatedAt            string //
	DeletedAt            string //
	WelcomeMessage       string //
}

// youbanTwoWayBotBotColumns holds the columns for the table hg_youban_two_way_bot_bot.
var youbanTwoWayBotBotColumns = YoubanTwoWayBotBotColumns{
	Id:                   "id",
	TenantId:             "tenant_id",
	AccountId:            "account_id",
	TgAccountId:          "tg_account_id",
	Name:                 "name",
	BotToken:             "bot_token",
	BotUserId:            "bot_user_id",
	BotUsername:          "bot_username",
	SupergroupId:         "supergroup_id",
	SupergroupAccessHash: "supergroup_access_hash",
	SupergroupTitle:      "supergroup_title",
	InviteLink:           "invite_link",
	SetupStatus:          "setup_status",
	WebhookStatus:        "webhook_status",
	Status:               "status",
	ErrorMessage:         "error_message",
	LastSetupAt:          "last_setup_at",
	LastWebhookAt:        "last_webhook_at",
	CreatedAt:            "created_at",
	UpdatedAt:            "updated_at",
	DeletedAt:            "deleted_at",
	WelcomeMessage:       "welcome_message",
}

// NewYoubanTwoWayBotBotDao creates and returns a new DAO object for table data access.
func NewYoubanTwoWayBotBotDao(handlers ...gdb.ModelHandler) *YoubanTwoWayBotBotDao {
	return &YoubanTwoWayBotBotDao{
		group:    "default",
		table:    "hg_youban_two_way_bot_bot",
		columns:  youbanTwoWayBotBotColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanTwoWayBotBotDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanTwoWayBotBotDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanTwoWayBotBotDao) Columns() YoubanTwoWayBotBotColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanTwoWayBotBotDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanTwoWayBotBotDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanTwoWayBotBotDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
