// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishTaskDao is the data access object for the table hg_youban_publish_task.
type YoubanPublishTaskDao struct {
	table    string                   // table is the underlying table name of the DAO.
	group    string                   // group is the database configuration group name of the current DAO.
	columns  YoubanPublishTaskColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler       // handlers for customized model modification.
}

// YoubanPublishTaskColumns defines and stores column names for the table hg_youban_publish_task.
type YoubanPublishTaskColumns struct {
	Id              string //
	TenantId        string //
	MerchantId      string //
	AccountId       string //
	ProfileId       string //
	ClientRequestId string //
	Title           string //
	Province        string //
	City            string //
	PlainText       string //
	MediaCount      string //
	TgPushEnabled   string //
	TgStatus        string //
	Status          string //
	ErrorMessage    string //
	SubmittedAt     string //
	PublishedAt     string //
	CreatedBy       string //
	UpdatedBy       string //
	DeletedBy       string //
	CreatedAt       string //
	UpdatedAt       string //
	DeletedAt       string //
	ChannelIdJson   string //
	CustomerRemark  string //
	AntiScanEnabled string //
	TgOperationNo   string //
}

// youbanPublishTaskColumns holds the columns for the table hg_youban_publish_task.
var youbanPublishTaskColumns = YoubanPublishTaskColumns{
	Id:              "id",
	TenantId:        "tenant_id",
	MerchantId:      "merchant_id",
	AccountId:       "account_id",
	ProfileId:       "profile_id",
	ClientRequestId: "client_request_id",
	Title:           "title",
	Province:        "province",
	City:            "city",
	PlainText:       "plain_text",
	MediaCount:      "media_count",
	TgPushEnabled:   "tg_push_enabled",
	TgStatus:        "tg_status",
	Status:          "status",
	ErrorMessage:    "error_message",
	SubmittedAt:     "submitted_at",
	PublishedAt:     "published_at",
	CreatedBy:       "created_by",
	UpdatedBy:       "updated_by",
	DeletedBy:       "deleted_by",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
	DeletedAt:       "deleted_at",
	ChannelIdJson:   "channel_id_json",
	CustomerRemark:  "customer_remark",
	AntiScanEnabled: "anti_scan_enabled",
	TgOperationNo:   "tg_operation_no",
}

// NewYoubanPublishTaskDao creates and returns a new DAO object for table data access.
func NewYoubanPublishTaskDao(handlers ...gdb.ModelHandler) *YoubanPublishTaskDao {
	return &YoubanPublishTaskDao{
		group:    "default",
		table:    "hg_youban_publish_task",
		columns:  youbanPublishTaskColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishTaskDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishTaskDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishTaskDao) Columns() YoubanPublishTaskColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishTaskDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishTaskDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishTaskDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
