// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishMaterialImportGroupDao is the data access object for the table hg_youban_publish_material_import_group.
type YoubanPublishMaterialImportGroupDao struct {
	table    string                                  // table is the underlying table name of the DAO.
	group    string                                  // group is the database configuration group name of the current DAO.
	columns  YoubanPublishMaterialImportGroupColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                      // handlers for customized model modification.
}

// YoubanPublishMaterialImportGroupColumns defines and stores column names for the table hg_youban_publish_material_import_group.
type YoubanPublishMaterialImportGroupColumns struct {
	Id               string //
	TaskId           string //
	TenantId         string //
	AccountId        string //
	SourceChatId     string //
	SourceGroupedId  string //
	SourceMessageIds string //
	SourceUniqueKey  string //
	Title            string //
	Nickname         string //
	ProfileNo        string //
	RawText          string //
	ProfileText      string //
	VerifyText       string //
	MediaJson        string //
	MediaTotal       string //
	MediaDone        string //
	MediaFailed      string //
	ProfileId        string //
	TaskProfileId    string //
	Status           string //
	ErrorMessage     string //
	MessageAt        string //
	CreatedAt        string //
	UpdatedAt        string //
}

// youbanPublishMaterialImportGroupColumns holds the columns for the table hg_youban_publish_material_import_group.
var youbanPublishMaterialImportGroupColumns = YoubanPublishMaterialImportGroupColumns{
	Id:               "id",
	TaskId:           "task_id",
	TenantId:         "tenant_id",
	AccountId:        "account_id",
	SourceChatId:     "source_chat_id",
	SourceGroupedId:  "source_grouped_id",
	SourceMessageIds: "source_message_ids",
	SourceUniqueKey:  "source_unique_key",
	Title:            "title",
	Nickname:         "nickname",
	ProfileNo:        "profile_no",
	RawText:          "raw_text",
	ProfileText:      "profile_text",
	VerifyText:       "verify_text",
	MediaJson:        "media_json",
	MediaTotal:       "media_total",
	MediaDone:        "media_done",
	MediaFailed:      "media_failed",
	ProfileId:        "profile_id",
	TaskProfileId:    "task_profile_id",
	Status:           "status",
	ErrorMessage:     "error_message",
	MessageAt:        "message_at",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
}

// NewYoubanPublishMaterialImportGroupDao creates and returns a new DAO object for table data access.
func NewYoubanPublishMaterialImportGroupDao(handlers ...gdb.ModelHandler) *YoubanPublishMaterialImportGroupDao {
	return &YoubanPublishMaterialImportGroupDao{
		group:    "default",
		table:    "hg_youban_publish_material_import_group",
		columns:  youbanPublishMaterialImportGroupColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishMaterialImportGroupDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishMaterialImportGroupDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishMaterialImportGroupDao) Columns() YoubanPublishMaterialImportGroupColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishMaterialImportGroupDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishMaterialImportGroupDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishMaterialImportGroupDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
