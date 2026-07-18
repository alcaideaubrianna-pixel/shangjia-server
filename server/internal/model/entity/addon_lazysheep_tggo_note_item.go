// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoNoteItem is the golang structure for table addon_lazysheep_tggo_note_item.
type AddonLazysheepTggoNoteItem struct {
	Id           int64       `json:"id"           orm:"id"            description:"主键"`
	NoteId       int64       `json:"noteId"       orm:"note_id"       description:"笔记ID"`
	ItemIndex    int         `json:"itemIndex"    orm:"item_index"    description:"序号"`
	ItemType     string      `json:"itemType"     orm:"item_type"     description:"项目类型"`
	Title        string      `json:"title"        orm:"title"         description:"标题"`
	SubTitle     string      `json:"subTitle"     orm:"sub_title"     description:"副标题"`
	Content      string      `json:"content"      orm:"content"       description:"内容"`
	Duration     int         `json:"duration"     orm:"duration"      description:"时长"`
	AspectRatio  float64     `json:"aspectRatio"  orm:"aspect_ratio"  description:"宽高比"`
	VerifyVideo  int         `json:"verifyVideo"  orm:"verify_video"  description:"验证视频"`
	AttachmentId int64       `json:"attachmentId" orm:"attachment_id" description:"附件ID"`
	PreviewUrl   string      `json:"previewUrl"   orm:"preview_url"   description:"预览地址"`
	LocalPath    string      `json:"localPath"    orm:"local_path"    description:"本地路径"`
	TgFileId     string      `json:"tgFileId"     orm:"tg_file_id"    description:"Telegram fileId"`
	Status       int         `json:"status"       orm:"status"        description:"状态"`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"    description:"创建时间"`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"    description:"更新时间"`
	DeletedAt    *gtime.Time `json:"deletedAt"    orm:"deleted_at"    description:"删除时间"`
}
