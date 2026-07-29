// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectContentDao is the data access object for the table hg_youban_publish_collect_content.
type YoubanPublishCollectContentDao struct {
	table    string                             // table is the underlying table name of the DAO.
	group    string                             // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectContentColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                 // handlers for customized model modification.
}

// YoubanPublishCollectContentColumns defines and stores column names for the table hg_youban_publish_collect_content.
type YoubanPublishCollectContentColumns struct {
	Id             string //
	TenantId       string //
	AccountId      string //
	FirstEventId   string //
	LastEventId    string //
	SourceType     string //
	RawText        string //
	NormalizedText string //
	MediaCount     string //
	MediaJson      string //
	TextHash       string //
	DedupeKey      string //
	DuplicateTotal string //
	Status         string //
	FirstSeenAt    string //
	PreviousSeenAt string //
	LastSeenAt     string //
	CreatedAt      string //
	UpdatedAt      string //
}

// youbanPublishCollectContentColumns holds the columns for the table hg_youban_publish_collect_content.
var youbanPublishCollectContentColumns = YoubanPublishCollectContentColumns{
	Id:             "id",
	TenantId:       "tenant_id",
	AccountId:      "account_id",
	FirstEventId:   "first_event_id",
	LastEventId:    "last_event_id",
	SourceType:     "source_type",
	RawText:        "raw_text",
	NormalizedText: "normalized_text",
	MediaCount:     "media_count",
	MediaJson:      "media_json",
	TextHash:       "text_hash",
	DedupeKey:      "dedupe_key",
	DuplicateTotal: "duplicate_total",
	Status:         "status",
	FirstSeenAt:    "first_seen_at",
	PreviousSeenAt: "previous_seen_at",
	LastSeenAt:     "last_seen_at",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
}

// NewYoubanPublishCollectContentDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectContentDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectContentDao {
	return &YoubanPublishCollectContentDao{
		group:    "default",
		table:    "hg_youban_publish_collect_content",
		columns:  youbanPublishCollectContentColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectContentDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectContentDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectContentDao) Columns() YoubanPublishCollectContentColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectContentDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectContentDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectContentDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
