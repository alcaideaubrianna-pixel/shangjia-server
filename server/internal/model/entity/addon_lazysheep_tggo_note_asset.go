// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoNoteAsset is the golang structure for table addon_lazysheep_tggo_note_asset.
type AddonLazysheepTggoNoteAsset struct {
	Id            int64       `json:"id"            orm:"id"             description:"主键"`
	NoteId        int64       `json:"noteId"        orm:"note_id"        description:"笔记ID"`
	BotId         int64       `json:"botId"         orm:"bot_id"         description:"机器人ID"`
	ItemId        int64       `json:"itemId"        orm:"item_id"        description:"笔记项ID"`
	AssetType     string      `json:"assetType"     orm:"asset_type"     description:"资源类型:image/video/verify_video"`
	SourceUrl     string      `json:"sourceUrl"     orm:"source_url"     description:"源地址"`
	AttachmentId  int64       `json:"attachmentId"  orm:"attachment_id"  description:"附件ID"`
	PreviewUrl    string      `json:"previewUrl"    orm:"preview_url"    description:"预览地址"`
	LocalPath     string      `json:"localPath"     orm:"local_path"     description:"本地路径"`
	MimeType      string      `json:"mimeType"      orm:"mime_type"      description:"MIME类型"`
	FileSize      int64       `json:"fileSize"      orm:"file_size"      description:"文件大小"`
	Duration      int         `json:"duration"      orm:"duration"       description:"时长"`
	AspectRatio   float64     `json:"aspectRatio"   orm:"aspect_ratio"   description:"宽高比"`
	TgFileId      string      `json:"tgFileId"      orm:"tg_file_id"     description:"Telegram fileId"`
	ConvertStatus int         `json:"convertStatus" orm:"convert_status" description:"转换状态"`
	Sort          int         `json:"sort"          orm:"sort"           description:"排序"`
	Status        int         `json:"status"        orm:"status"         description:"状态"`
	CreatedAt     *gtime.Time `json:"createdAt"     orm:"created_at"     description:"创建时间"`
	UpdatedAt     *gtime.Time `json:"updatedAt"     orm:"updated_at"     description:"更新时间"`
	DeletedAt     *gtime.Time `json:"deletedAt"     orm:"deleted_at"     description:"删除时间"`
}
