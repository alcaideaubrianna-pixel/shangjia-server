// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishRelayPackageDao is the data access object for the table hg_youban_publish_relay_package.
type YoubanPublishRelayPackageDao struct {
	table    string                           // table is the underlying table name of the DAO.
	group    string                           // group is the database configuration group name of the current DAO.
	columns  YoubanPublishRelayPackageColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler               // handlers for customized model modification.
}

// YoubanPublishRelayPackageColumns defines and stores column names for the table hg_youban_publish_relay_package.
type YoubanPublishRelayPackageColumns struct {
	Id             string //
	TenantId       string //
	JobId          string //
	BotId          string //
	TelegramUserId string //
	RelayChannelId string //
	Generation     string //
	Status         string //
	Payload        string //
	Messages       string //
	AttemptId      string //
	TaskId         string //
	LeaseUntil     string //
	NextRetryAt    string //
	LastError      string //
	CreatedAt      string //
	UpdatedAt      string //
}

// youbanPublishRelayPackageColumns holds the columns for the table hg_youban_publish_relay_package.
var youbanPublishRelayPackageColumns = YoubanPublishRelayPackageColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	JobId:          "job_id",
	BotId:          "bot_id",
	TelegramUserId: "telegram_user_id",
	RelayChannelId: "relay_channel_id",
	Generation:     "generation",
	Status:         "status",
	Payload:        "payload",
	Messages:       "messages",
	AttemptId:      "attempt_id",
	TaskId:         "task_id",
	LeaseUntil:     "lease_until",
	NextRetryAt:    "next_retry_at",
	LastError:      "last_error",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
}

// NewYoubanPublishRelayPackageDao creates and returns a new DAO object for table data access.
func NewYoubanPublishRelayPackageDao(handlers ...gdb.ModelHandler) *YoubanPublishRelayPackageDao {
	return &YoubanPublishRelayPackageDao{
		group:    "default",
		table:    "hg_youban_publish_relay_package",
		columns:  youbanPublishRelayPackageColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishRelayPackageDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishRelayPackageDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishRelayPackageDao) Columns() YoubanPublishRelayPackageColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishRelayPackageDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishRelayPackageDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishRelayPackageDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
