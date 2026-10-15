// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanBotRegistryDao is the data access object for the table hg_youban_bot_registry.
type YoubanBotRegistryDao struct {
	table    string                   // table is the underlying table name of the DAO.
	group    string                   // group is the database configuration group name of the current DAO.
	columns  YoubanBotRegistryColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler       // handlers for customized model modification.
}

// YoubanBotRegistryColumns defines and stores column names for the table hg_youban_bot_registry.
type YoubanBotRegistryColumns struct {
	Id                   string //
	TenantId             string //
	AccountId            string //
	BotType              string //
	BotName              string //
	BotUsername          string //
	BotToken             string //
	BotUserId            string //
	IsOfficial           string //
	IsDefault            string //
	RunMode              string //
	WebhookUrl           string //
	Remark               string //
	Status               string //
	CreatedBy            string //
	UpdatedBy            string //
	DeletedBy            string //
	TgAccountId          string //
	WelcomeMessage       string //
	SupergroupId         string //
	SupergroupAccessHash string //
	SupergroupTitle      string //
	InviteLink           string //
	SetupStatus          string //
	WebhookStatus        string //
	ErrorMessage         string //
	LastSetupAt          string //
	LastWebhookAt        string //
	CreatedAt            string //
	UpdatedAt            string //
	DeletedAt            string //
}

// youbanBotRegistryColumns holds the columns for the table hg_youban_bot_registry.
var youbanBotRegistryColumns = YoubanBotRegistryColumns{
	Id:                   "id",
	TenantId:             "tenant_id",
	AccountId:            "account_id",
	BotType:              "bot_type",
	BotName:              "bot_name",
	BotUsername:          "bot_username",
	BotToken:             "bot_token",
	BotUserId:            "bot_user_id",
	IsOfficial:           "is_official",
	IsDefault:            "is_default",
	RunMode:              "run_mode",
	WebhookUrl:           "webhook_url",
	Remark:               "remark",
	Status:               "status",
	CreatedBy:            "created_by",
	UpdatedBy:            "updated_by",
	DeletedBy:            "deleted_by",
	TgAccountId:          "tg_account_id",
	WelcomeMessage:       "welcome_message",
	SupergroupId:         "supergroup_id",
	SupergroupAccessHash: "supergroup_access_hash",
	SupergroupTitle:      "supergroup_title",
	InviteLink:           "invite_link",
	SetupStatus:          "setup_status",
	WebhookStatus:        "webhook_status",
	ErrorMessage:         "error_message",
	LastSetupAt:          "last_setup_at",
	LastWebhookAt:        "last_webhook_at",
	CreatedAt:            "created_at",
	UpdatedAt:            "updated_at",
	DeletedAt:            "deleted_at",
}

// NewYoubanBotRegistryDao creates and returns a new DAO object for table data access.
func NewYoubanBotRegistryDao(handlers ...gdb.ModelHandler) *YoubanBotRegistryDao {
	return &YoubanBotRegistryDao{
		group:    "default",
		table:    "hg_youban_bot_registry",
		columns:  youbanBotRegistryColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanBotRegistryDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanBotRegistryDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanBotRegistryDao) Columns() YoubanBotRegistryColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanBotRegistryDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanBotRegistryDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanBotRegistryDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
