// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoNoteItem is the golang structure of table hg_addon_lazysheep_tggo_note_item for DAO operations like Where/Data.
type AddonLazysheepTggoNoteItem struct {
	g.Meta       `orm:"table:hg_addon_lazysheep_tggo_note_item, do:true"`
	Id           any         // 主键
	NoteId       any         // 笔记ID
	ItemIndex    any         // 序号
	ItemType     any         // 项目类型
	Title        any         // 标题
	SubTitle     any         // 副标题
	Content      any         // 内容
	Duration     any         // 时长
	AspectRatio  any         // 宽高比
	VerifyVideo  any         // 验证视频
	AttachmentId any         // 附件ID
	PreviewUrl   any         // 预览地址
	LocalPath    any         // 本地路径
	TgFileId     any         // Telegram fileId
	Status       any         // 状态
	CreatedAt    *gtime.Time // 创建时间
	UpdatedAt    *gtime.Time // 更新时间
	DeletedAt    *gtime.Time // 删除时间
}
