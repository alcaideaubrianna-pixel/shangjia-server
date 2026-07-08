// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishImportMatchCandidateDao is the data access object for the table hg_youban_publish_import_match_candidate.
type YoubanPublishImportMatchCandidateDao struct {
	table    string                                   // table is the underlying table name of the DAO.
	group    string                                   // group is the database configuration group name of the current DAO.
	columns  YoubanPublishImportMatchCandidateColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                       // handlers for customized model modification.
}

// YoubanPublishImportMatchCandidateColumns defines and stores column names for the table hg_youban_publish_import_match_candidate.
type YoubanPublishImportMatchCandidateColumns struct {
	Id             string //
	MatchRunId     string //
	TenantId       string //
	ChannelId      string //
	GroupKey       string //
	MediaGroupId   string //
	FirstMessageId string //
	LastMessageId  string //
	MessageDate    string //
	CaptionText    string //
	MediaCount     string //
	MediaTypes     string //
	PreviewJson    string //
	CreatedAt      string //
	UpdatedAt      string //
	DeletedAt      string //
}

// youbanPublishImportMatchCandidateColumns holds the columns for the table hg_youban_publish_import_match_candidate.
var youbanPublishImportMatchCandidateColumns = YoubanPublishImportMatchCandidateColumns{
	Id:             "id",
	MatchRunId:     "match_run_id",
	TenantId:       "tenant_id",
	ChannelId:      "channel_id",
	GroupKey:       "group_key",
	MediaGroupId:   "media_group_id",
	FirstMessageId: "first_message_id",
	LastMessageId:  "last_message_id",
	MessageDate:    "message_date",
	CaptionText:    "caption_text",
	MediaCount:     "media_count",
	MediaTypes:     "media_types",
	PreviewJson:    "preview_json",
	CreatedAt:      "created_at",
	UpdatedAt:      "updated_at",
	DeletedAt:      "deleted_at",
}

// NewYoubanPublishImportMatchCandidateDao creates and returns a new DAO object for table data access.
func NewYoubanPublishImportMatchCandidateDao(handlers ...gdb.ModelHandler) *YoubanPublishImportMatchCandidateDao {
	return &YoubanPublishImportMatchCandidateDao{
		group:    "default",
		table:    "hg_youban_publish_import_match_candidate",
		columns:  youbanPublishImportMatchCandidateColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishImportMatchCandidateDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishImportMatchCandidateDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishImportMatchCandidateDao) Columns() YoubanPublishImportMatchCandidateColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishImportMatchCandidateDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishImportMatchCandidateDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishImportMatchCandidateDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
