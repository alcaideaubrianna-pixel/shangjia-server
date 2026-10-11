// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishRelayMediaDao is the data access object for the table hg_youban_publish_relay_media.
type YoubanPublishRelayMediaDao struct {
	table    string                         // table is the underlying table name of the DAO.
	group    string                         // group is the database configuration group name of the current DAO.
	columns  YoubanPublishRelayMediaColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler             // handlers for customized model modification.
}

// YoubanPublishRelayMediaColumns defines and stores column names for the table hg_youban_publish_relay_media.
type YoubanPublishRelayMediaColumns struct {
	Id             string //
	TenantId       string //
	MediaId        string //
	Purpose        string //
	MediaVersion   string //
	RelayChannelId string //
	RelayChatId    string //
	MessageId      string //
	FileId         string //
	Status         string //
	LeaseToken     string //
	LeaseUntil     string //
	LastError      string //
	ExpiresAt      string //
	CreatedAt      string //
	UpdatedAt      string //
}

// youbanPublishRelayMediaColumns holds the columns for the table hg_youban_publish_relay_media.
var youbanPublishRelayMediaColumns = YoubanPublishRelayMediaColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	MediaId:        "media_id",
	Purpose:        "purpose",
	MediaVersion:   "media_version",
	RelayChannelId: "relay_channel_id",
	RelayChatId:    "relay_chat_id",
	MessageId:      "message_id",
	FileId:         "file_id",
	Status:         "status",
	LeaseToken:     "lease_token",
	LeaseUntil:     "lease_until",
	LastError:      "last_error",
	ExpiresAt:      "expires_at",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
}

// NewYoubanPublishRelayMediaDao creates and returns a new DAO object for table data access.
func NewYoubanPublishRelayMediaDao(handlers ...gdb.ModelHandler) *YoubanPublishRelayMediaDao {
	return &YoubanPublishRelayMediaDao{
		group:    "default",
		table:    "hg_youban_publish_relay_media",
		columns:  youbanPublishRelayMediaColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishRelayMediaDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishRelayMediaDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishRelayMediaDao) Columns() YoubanPublishRelayMediaColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishRelayMediaDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishRelayMediaDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishRelayMediaDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
