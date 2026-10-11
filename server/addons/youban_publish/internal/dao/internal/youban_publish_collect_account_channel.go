// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectAccountChannelDao is the data access object for the table hg_youban_publish_collect_account_channel.
type YoubanPublishCollectAccountChannelDao struct {
	table    string                                    // table is the underlying table name of the DAO.
	group    string                                    // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectAccountChannelColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                        // handlers for customized model modification.
}

// YoubanPublishCollectAccountChannelColumns defines and stores column names for the table hg_youban_publish_collect_account_channel.
type YoubanPublishCollectAccountChannelColumns struct {
	Id             string //
	TenantId       string //
	AccountId      string //
	ChannelId      string //
	SourceId       string //
	CollectEnabled string //
	Status         string //
	ApprovalStatus string //
	ApprovedAt     string //
	DisabledAt     string //
	DisabledReason string //
	CreatedAt      string //
	UpdatedAt      string //
	DeletedAt      string //
}

// youbanPublishCollectAccountChannelColumns holds the columns for the table hg_youban_publish_collect_account_channel.
var youbanPublishCollectAccountChannelColumns = YoubanPublishCollectAccountChannelColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	AccountId:      "account_id",
	ChannelId:      "channel_id",
	SourceId:       "source_id",
	CollectEnabled: "collect_enabled",
	Status:         "status",
	ApprovalStatus: "approval_status",
	ApprovedAt:     "approved_at",
	DisabledAt:     "disabled_at",
	DisabledReason: "disabled_reason",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
	DeletedAt:      "deleted_at",
}

// NewYoubanPublishCollectAccountChannelDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectAccountChannelDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectAccountChannelDao {
	return &YoubanPublishCollectAccountChannelDao{
		group:    "default",
		table:    "hg_youban_publish_collect_account_channel",
		columns:  youbanPublishCollectAccountChannelColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectAccountChannelDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectAccountChannelDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectAccountChannelDao) Columns() YoubanPublishCollectAccountChannelColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectAccountChannelDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectAccountChannelDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectAccountChannelDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
