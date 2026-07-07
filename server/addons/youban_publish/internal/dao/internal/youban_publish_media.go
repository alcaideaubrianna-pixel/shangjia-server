// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// YoubanPublishMediaDao is the data access object for the table hg_youban_publish_media.
type YoubanPublishMediaDao struct {
	table    string                    // table is the underlying table name of the DAO.
	group    string                    // group is the database configuration group name of the current DAO.
	columns  YoubanPublishMediaColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler        // handlers for customized model modification.
}

// YoubanPublishMediaColumns defines and stores column names for the table hg_youban_publish_media.
type YoubanPublishMediaColumns struct {
	Id                   string //
	TenantId             string //
	MerchantId           string //
	AccountId            string //
	TaskId               string //
	ProfileId            string //
	AttachmentId         string //
	MediaType            string //
	Name                 string //
	FileUrl              string //
	StoragePath          string //
	MimeType             string //
	Md5                  string //
	Size                 string //
	SortIndex            string //
	Status               string //
	CreatedBy            string //
	UpdatedBy            string //
	DeletedBy            string //
	CreatedAt            string //
	UpdatedAt            string //
	DeletedAt            string //
	PerceptualHash       string //
	Purpose              string //
	PosterUrl            string //
	TgFileId             string //
	TgThumbFileId        string //
	PosterStoragePath    string //
	OriginalAttachmentId string //
	OriginalFileUrl      string //
	OriginalStoragePath  string //
	EditedAttachmentId   string //
	EditedFileUrl        string //
	EditedStoragePath    string //
	EditConfigJson       string //
	EditStatus           string //
	TgCacheAssetHash     string //
	TgCacheStatus        string //
}

// youbanPublishMediaColumns holds the columns for the table hg_youban_publish_media.
var youbanPublishMediaColumns = YoubanPublishMediaColumns{
	Id:                   "id",
	TenantId:             "tenant_id",
	MerchantId:           "merchant_id",
	AccountId:            "account_id",
	TaskId:               "task_id",
	ProfileId:            "profile_id",
	AttachmentId:         "attachment_id",
	MediaType:            "media_type",
	Name:                 "name",
	FileUrl:              "file_url",
	StoragePath:          "storage_path",
	MimeType:             "mime_type",
	Md5:                  "md5",
	Size:                 "size",
	SortIndex:            "sort_index",
	Status:               "status",
	CreatedBy:            "created_by",
	UpdatedBy:            "updated_by",
	DeletedBy:            "deleted_by",
	CreatedAt:            "created_at",
	UpdatedAt:            "updated_at",
	DeletedAt:            "deleted_at",
	PerceptualHash:       "perceptual_hash",
	Purpose:              "purpose",
	PosterUrl:            "poster_url",
	TgFileId:             "tg_file_id",
	TgThumbFileId:        "tg_thumb_file_id",
	PosterStoragePath:    "poster_storage_path",
	OriginalAttachmentId: "original_attachment_id",
	OriginalFileUrl:      "original_file_url",
	OriginalStoragePath:  "original_storage_path",
	EditedAttachmentId:   "edited_attachment_id",
	EditedFileUrl:        "edited_file_url",
	EditedStoragePath:    "edited_storage_path",
	EditConfigJson:       "edit_config_json",
	EditStatus:           "edit_status",
	TgCacheAssetHash:     "tg_cache_asset_hash",
	TgCacheStatus:        "tg_cache_status",
}

// NewYoubanPublishMediaDao creates and returns a new DAO object for table data access.
func NewYoubanPublishMediaDao(handlers ...gdb.ModelHandler) *YoubanPublishMediaDao {
	return &YoubanPublishMediaDao{
		group:    "default",
		table:    "hg_youban_publish_media",
		columns:  youbanPublishMediaColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *YoubanPublishMediaDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *YoubanPublishMediaDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *YoubanPublishMediaDao) Columns() YoubanPublishMediaColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *YoubanPublishMediaDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *YoubanPublishMediaDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *YoubanPublishMediaDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
