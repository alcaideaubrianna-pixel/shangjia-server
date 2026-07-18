// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// AddonLazysheepTggoNoteItemDao is the data access object for the table hg_addon_lazysheep_tggo_note_item.
type AddonLazysheepTggoNoteItemDao struct {
	table    string                            // table is the underlying table name of the DAO.
	group    string                            // group is the database configuration group name of the current DAO.
	columns  AddonLazysheepTggoNoteItemColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler                // handlers for customized model modification.
}

// AddonLazysheepTggoNoteItemColumns defines and stores column names for the table hg_addon_lazysheep_tggo_note_item.
type AddonLazysheepTggoNoteItemColumns struct {
	Id           string // 主键
	NoteId       string // 笔记ID
	ItemIndex    string // 序号
	ItemType     string // 项目类型
	Title        string // 标题
	SubTitle     string // 副标题
	Content      string // 内容
	Duration     string // 时长
	AspectRatio  string // 宽高比
	VerifyVideo  string // 验证视频
	AttachmentId string // 附件ID
	PreviewUrl   string // 预览地址
	LocalPath    string // 本地路径
	TgFileId     string // Telegram fileId
	Status       string // 状态
	CreatedAt    string // 创建时间
	UpdatedAt    string // 更新时间
	DeletedAt    string // 删除时间
}

// addonLazysheepTggoNoteItemColumns holds the columns for the table hg_addon_lazysheep_tggo_note_item.
var addonLazysheepTggoNoteItemColumns = AddonLazysheepTggoNoteItemColumns{
	Id:           "id",
	NoteId:       "note_id",
	ItemIndex:    "item_index",
	ItemType:     "item_type",
	Title:        "title",
	SubTitle:     "sub_title",
	Content:      "content",
	Duration:     "duration",
	AspectRatio:  "aspect_ratio",
	VerifyVideo:  "verify_video",
	AttachmentId: "attachment_id",
	PreviewUrl:   "preview_url",
	LocalPath:    "local_path",
	TgFileId:     "tg_file_id",
	Status:       "status",
	CreatedAt:    "created_at",
	UpdatedAt:    "updated_at",
	DeletedAt:    "deleted_at",
}

// NewAddonLazysheepTggoNoteItemDao creates and returns a new DAO object for table data access.
func NewAddonLazysheepTggoNoteItemDao(handlers ...gdb.ModelHandler) *AddonLazysheepTggoNoteItemDao {
	return &AddonLazysheepTggoNoteItemDao{
		group:    "default",
		table:    "hg_addon_lazysheep_tggo_note_item",
		columns:  addonLazysheepTggoNoteItemColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AddonLazysheepTggoNoteItemDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AddonLazysheepTggoNoteItemDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AddonLazysheepTggoNoteItemDao) Columns() AddonLazysheepTggoNoteItemColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AddonLazysheepTggoNoteItemDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AddonLazysheepTggoNoteItemDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AddonLazysheepTggoNoteItemDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
