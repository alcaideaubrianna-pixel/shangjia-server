// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// AddonLazysheepTggoNoteAssetDao is the data access object for the table hg_addon_lazysheep_tggo_note_asset.
type AddonLazysheepTggoNoteAssetDao struct {
	table    string                             // table is the underlying table name of the DAO.
	group    string                             // group is the database configuration group name of the current DAO.
	columns  AddonLazysheepTggoNoteAssetColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                 // handlers for customized model modification.
}

// AddonLazysheepTggoNoteAssetColumns defines and stores column names for the table hg_addon_lazysheep_tggo_note_asset.
type AddonLazysheepTggoNoteAssetColumns struct {
	Id            string // 主键
	NoteId        string // 笔记ID
	BotId         string // 机器人ID
	ItemId        string // 笔记项ID
	AssetType     string // 资源类型:image/video/verify_video
	SourceUrl     string // 源地址
	AttachmentId  string // 附件ID
	PreviewUrl    string // 预览地址
	LocalPath     string // 本地路径
	MimeType      string // MIME类型
	FileSize      string // 文件大小
	Duration      string // 时长
	AspectRatio   string // 宽高比
	TgFileId      string // Telegram fileId
	ConvertStatus string // 转换状态
	Sort          string // 排序
	Status        string // 状态
	CreatedAt     string // 创建时间
	UpdatedAt     string // 更新时间
	DeletedAt     string // 删除时间
}

// addonLazysheepTggoNoteAssetColumns holds the columns for the table hg_addon_lazysheep_tggo_note_asset.
var addonLazysheepTggoNoteAssetColumns = AddonLazysheepTggoNoteAssetColumns{
	Id:            "id",
	NoteId:        "note_id",
	BotId:         "bot_id",
	ItemId:        "item_id",
	AssetType:     "asset_type",
	SourceUrl:     "source_url",
	AttachmentId:  "attachment_id",
	PreviewUrl:    "preview_url",
	LocalPath:     "local_path",
	MimeType:      "mime_type",
	FileSize:      "file_size",
	Duration:      "duration",
	AspectRatio:   "aspect_ratio",
	TgFileId:      "tg_file_id",
	ConvertStatus: "convert_status",
	Sort:          "sort",
	Status:        "status",
	CreatedAt:     "created_at",
	UpdatedAt:     "updated_at",
	DeletedAt:     "deleted_at",
}

// NewAddonLazysheepTggoNoteAssetDao creates and returns a new DAO object for table data access.
func NewAddonLazysheepTggoNoteAssetDao(handlers ...gdb.ModelHandler) *AddonLazysheepTggoNoteAssetDao {
	return &AddonLazysheepTggoNoteAssetDao{
		group:    "default",
		table:    "hg_addon_lazysheep_tggo_note_asset",
		columns:  addonLazysheepTggoNoteAssetColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AddonLazysheepTggoNoteAssetDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AddonLazysheepTggoNoteAssetDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AddonLazysheepTggoNoteAssetDao) Columns() AddonLazysheepTggoNoteAssetColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AddonLazysheepTggoNoteAssetDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AddonLazysheepTggoNoteAssetDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AddonLazysheepTggoNoteAssetDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
