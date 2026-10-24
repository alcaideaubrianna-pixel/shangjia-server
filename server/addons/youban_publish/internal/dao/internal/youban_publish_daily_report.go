// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishDailyReportDao is the data access object for the table hg_youban_publish_daily_report.
type YoubanPublishDailyReportDao struct {
	table    string                          // table is the underlying table name of the DAO.
	group    string                          // group is the database configuration group name of the current DAO.
	columns  YoubanPublishDailyReportColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler              // handlers for customized model modification.
}

// YoubanPublishDailyReportColumns defines and stores column names for the table hg_youban_publish_daily_report.
type YoubanPublishDailyReportColumns struct {
	Id                  string //
	TenantId            string //
	AccountId           string //
	ReportDate          string //
	CollectedTotal      string //
	PublishSuccess      string //
	PublishFailed       string //
	PublishProcessing   string //
	SourceBreakdownJson string //
	GeneratedAt         string //
	NotifyStatus        string //
	NotifyRetryCount    string //
	NotifyNextRetryAt   string //
	NotifyLeaseUntil    string //
	NotifyToken         string //
	NotifyCursor        string //
	NotifyError         string //
}

// youbanPublishDailyReportColumns holds the columns for the table hg_youban_publish_daily_report.
var youbanPublishDailyReportColumns = YoubanPublishDailyReportColumns{
	Id:                  "id",
	TenantId:            "tenant_id",
	AccountId:           "account_id",
	ReportDate:          "report_date",
	CollectedTotal:      "collected_total",
	PublishSuccess:      "publish_success",
	PublishFailed:       "publish_failed",
	PublishProcessing:   "publish_processing",
	SourceBreakdownJson: "source_breakdown_json",
	GeneratedAt:         "generated_at",
	NotifyStatus:        "notify_status",
	NotifyRetryCount:    "notify_retry_count",
	NotifyNextRetryAt:   "notify_next_retry_at",
	NotifyLeaseUntil:    "notify_lease_until",
	NotifyToken:         "notify_token",
	NotifyCursor:        "notify_cursor",
	NotifyError:         "notify_error",
}

// NewYoubanPublishDailyReportDao creates and returns a new DAO object for table data access.
func NewYoubanPublishDailyReportDao(handlers ...gdb.ModelHandler) *YoubanPublishDailyReportDao {
	return &YoubanPublishDailyReportDao{
		group:    "default",
		table:    "hg_youban_publish_daily_report",
		columns:  youbanPublishDailyReportColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishDailyReportDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishDailyReportDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishDailyReportDao) Columns() YoubanPublishDailyReportColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishDailyReportDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishDailyReportDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishDailyReportDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
