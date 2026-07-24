// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishNoteIndexDao is the data access object for the table hg_youban_publish_note_index.
type YoubanPublishNoteIndexDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  YoubanPublishNoteIndexColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// YoubanPublishNoteIndexColumns defines and stores column names for the table hg_youban_publish_note_index.
type YoubanPublishNoteIndexColumns struct {
	Id              string //
	TenantId        string //
	AccountId       string //
	ProfileId       string //
	TaskId          string //
	Uuid            string //
	ProfileNo       string //
	Title           string //
	Summary         string //
	PlainText       string //
	Tag             string //
	Province        string //
	City            string //
	Status          string //
	Visibility      string //
	ReviewStatus    string //
	TaskStatus      string //
	CoverMediaId    string //
	PublishedAt     string //
	SourceUpdatedAt string //
	CreatedAt       string //
	UpdatedAt       string //
	DeletedAt       string //
}

// youbanPublishNoteIndexColumns holds the columns for the table hg_youban_publish_note_index.
var youbanPublishNoteIndexColumns = YoubanPublishNoteIndexColumns{
	Id:              "id",
	TenantId:        "tenant_id",
	AccountId:       "account_id",
	ProfileId:       "profile_id",
	TaskId:          "task_id",
	Uuid:            "uuid",
	ProfileNo:       "profile_no",
	Title:           "title",
	Summary:         "summary",
	PlainText:       "plain_text",
	Tag:             "tag",
	Province:        "province",
	City:            "city",
	Status:          "status",
	Visibility:      "visibility",
	ReviewStatus:    "review_status",
	TaskStatus:      "task_status",
	CoverMediaId:    "cover_media_id",
	PublishedAt:     "published_at",
	SourceUpdatedAt: "source_updated_at",
	CreatedAt:       "created_at",
	UpdatedAt:       "updated_at",
	DeletedAt:       "deleted_at",
}

// NewYoubanPublishNoteIndexDao creates and returns a new DAO object for table data access.
func NewYoubanPublishNoteIndexDao(handlers ...gdb.ModelHandler) *YoubanPublishNoteIndexDao {
	return &YoubanPublishNoteIndexDao{
		group:    "default",
		table:    "hg_youban_publish_note_index",
		columns:  youbanPublishNoteIndexColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishNoteIndexDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishNoteIndexDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishNoteIndexDao) Columns() YoubanPublishNoteIndexColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishNoteIndexDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishNoteIndexDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishNoteIndexDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
