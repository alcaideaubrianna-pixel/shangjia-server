// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoNoteAsset is the golang structure of table hg_addon_lazysheep_tggo_note_asset for DAO operations like Where/Data.
type AddonLazysheepTggoNoteAsset struct {
	g.Meta        `orm:"table:hg_addon_lazysheep_tggo_note_asset, do:true"`
	Id            any         // 主键
	NoteId        any         // 笔记ID
	BotId         any         // 机器人ID
	ItemId        any         // 笔记项ID
	AssetType     any         // 资源类型:image/video/verify_video
	SourceUrl     any         // 源地址
	AttachmentId  any         // 附件ID
	PreviewUrl    any         // 预览地址
	LocalPath     any         // 本地路径
	MimeType      any         // MIME类型
	FileSize      any         // 文件大小
	Duration      any         // 时长
	AspectRatio   any         // 宽高比
	TgFileId      any         // Telegram fileId
	ConvertStatus any         // 转换状态
	Sort          any         // 排序
	Status        any         // 状态
	CreatedAt     *gtime.Time // 创建时间
	UpdatedAt     *gtime.Time // 更新时间
	DeletedAt     *gtime.Time // 删除时间
}
