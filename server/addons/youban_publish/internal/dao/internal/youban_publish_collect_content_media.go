// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishCollectContentMediaDao is the data access object for the table hg_youban_publish_collect_content_media.
type YoubanPublishCollectContentMediaDao struct {
	table    string                                  // table is the underlying table name of the DAO.
	group    string                                  // group is the database configuration group name of the current DAO.
	columns  YoubanPublishCollectContentMediaColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                      // handlers for customized model modification.
}

// YoubanPublishCollectContentMediaColumns defines and stores column names for the table hg_youban_publish_collect_content_media.
type YoubanPublishCollectContentMediaColumns struct {
	Id              string //
	TenantId        string //
	AccountId       string //
	ContentId       string //
	MediaType       string //
	SourceFileId    string //
	SourceUniqueKey string //
	FileMd5         string //
	FilePhash       string //
	SortIndex       string //
	Status          string //
	CreatedAt       string //
	UpdatedAt       string //
}

// youbanPublishCollectContentMediaColumns holds the columns for the table hg_youban_publish_collect_content_media.
var youbanPublishCollectContentMediaColumns = YoubanPublishCollectContentMediaColumns{
	Id:              "id",
	TenantId:        "tenant_id",
	AccountId:       "account_id",
	ContentId:       "content_id",
	MediaType:       "media_type",
	SourceFileId:    "source_file_id",
	SourceUniqueKey: "source_unique_key",
	FileMd5:         "file_md5",
	FilePhash:       "file_phash",
	SortIndex:       "sort_index",
	Status:          "status",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
}

// NewYoubanPublishCollectContentMediaDao creates and returns a new DAO object for table data access.
func NewYoubanPublishCollectContentMediaDao(handlers ...gdb.ModelHandler) *YoubanPublishCollectContentMediaDao {
	return &YoubanPublishCollectContentMediaDao{
		group:    "default",
		table:    "hg_youban_publish_collect_content_media",
		columns:  youbanPublishCollectContentMediaColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishCollectContentMediaDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishCollectContentMediaDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishCollectContentMediaDao) Columns() YoubanPublishCollectContentMediaColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishCollectContentMediaDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishCollectContentMediaDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishCollectContentMediaDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
