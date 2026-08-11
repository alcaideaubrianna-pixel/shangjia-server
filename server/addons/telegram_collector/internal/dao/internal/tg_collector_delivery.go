// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// TgCollectorDeliveryDao is the data access object for the table hg_tg_collector_delivery.
type TgCollectorDeliveryDao struct {
	table    string                     // table is the underlying table name of the DAO.
	group    string                     // group is the database configuration group name of the current DAO.
	columns  TgCollectorDeliveryColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler         // handlers for customized model modification.
}

// TgCollectorDeliveryColumns defines and stores column names for the table hg_tg_collector_delivery.
type TgCollectorDeliveryColumns struct {
	Id           string //
	TenantId     string //
	EventId      string //
	DeliveryKey  string //
	Status       string //
	Priority     string //
	Payload      string //
	AttemptCount string //
	NextRunAt    string //
	LeaseOwner   string //
	LeaseUntil   string //
	ErrorMessage string //
	CreatedAt    string //
	UpdatedAt    string //
}

// tgCollectorDeliveryColumns holds the columns for the table hg_tg_collector_delivery.
var tgCollectorDeliveryColumns = TgCollectorDeliveryColumns{
	Id:           "id",
	TenantId:     "tenant_id",
	EventId:      "event_id",
	DeliveryKey:  "delivery_key",
	Status:       "status",
	Priority:     "priority",
	Payload:      "payload",
	AttemptCount: "attempt_count",
	NextRunAt:    "next_run_at",
	LeaseOwner:   "lease_owner",
	LeaseUntil:   "lease_until",
	ErrorMessage: "error_message",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
}

// NewTgCollectorDeliveryDao creates and returns a new DAO object for table data access.
func NewTgCollectorDeliveryDao(handlers ...gdb.ModelHandler) *TgCollectorDeliveryDao {
	return &TgCollectorDeliveryDao{
		group:    "default",
		table:    "hg_tg_collector_delivery",
		columns:  tgCollectorDeliveryColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *TgCollectorDeliveryDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *TgCollectorDeliveryDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *TgCollectorDeliveryDao) Columns() TgCollectorDeliveryColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *TgCollectorDeliveryDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *TgCollectorDeliveryDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *TgCollectorDeliveryDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
