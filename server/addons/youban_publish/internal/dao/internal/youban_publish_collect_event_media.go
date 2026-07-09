// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectEventMediaDao is the data access object for the table hg_youban_publish_collect_event_media.
type YoubanPublishCollectEventMediaDao struct {
	table    string                                // table is the underlying table name of the DAO.
	group    string                                // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectEventMediaColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                    // handlers for customized model modification.
}

// YoubanPublishCollectEventMediaColumns defines and stores column names for the table hg_youban_publish_collect_event_media.
type YoubanPublishCollectEventMediaColumns struct {
	Id               string //
	TenantId         string //
	AccountId        string //
	SourceId         string //
	SourceType       string //
	EventId          string //
	SourceChatId     string //
	SourceMessageId  string //
	SourceGroupedId  string //
	SourceMediaKey   string //
	MediaType        string //
	SourceRefType    string //
	SourceFileId     string //
	SourceMessageRef string //
	BackupChannelId  string //
	BackupChatId     string //
	BackupMessageId  string //
	FileUrl          string //
	StoragePath      string //
	PosterUrl        string //
	MetaJson         string //
	SortIndex        string //
	CacheStatus      string //
	ErrorMessage     string //
	CreatedAt        string //
	UpdatedAt        string //
}

// youbanPublishCollectEventMediaColumns holds the columns for the table hg_youban_publish_collect_event_media.
var youbanPublishCollectEventMediaColumns = YoubanPublishCollectEventMediaColumns{
	Id:               "id",
	TenantId:         "tenant_id",
	AccountId:        "account_id",
	SourceId:         "source_id",
	SourceType:       "source_type",
	EventId:          "event_id",
	SourceChatId:     "source_chat_id",
	SourceMessageId:  "source_message_id",
	SourceGroupedId:  "source_grouped_id",
	SourceMediaKey:   "source_media_key",
	MediaType:        "media_type",
	SourceRefType:    "source_ref_type",
	SourceFileId:     "source_file_id",
	SourceMessageRef: "source_message_ref",
	BackupChannelId:  "backup_channel_id",
	BackupChatId:     "backup_chat_id",
	BackupMessageId:  "backup_message_id",
	FileUrl:          "file_url",
	StoragePath:      "storage_path",
	PosterUrl:        "poster_url",
	MetaJson:         "meta_json",
	SortIndex:        "sort_index",
	CacheStatus:      "cache_status",
	ErrorMessage:     "error_message",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
}

// NewYoubanPublishCollectEventMediaDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectEventMediaDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectEventMediaDao {
	return &YoubanPublishCollectEventMediaDao{
		group:    "default",
		table:    "hg_youban_publish_collect_event_media",
		columns:  youbanPublishCollectEventMediaColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectEventMediaDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectEventMediaDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectEventMediaDao) Columns() YoubanPublishCollectEventMediaColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectEventMediaDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectEventMediaDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectEventMediaDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
