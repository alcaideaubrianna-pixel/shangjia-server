// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMedia is the golang structure of table hg_youban_publish_media for DAO operations like Where/Data.
type YoubanPublishMedia struct {
	g.Meta               `orm:"table:hg_youban_publish_media, do:true"`
	Id                   any         //
	TenantId             any         //
	MerchantId           any         //
	AccountId            any         //
	TaskId               any         //
	ProfileId            any         //
	AttachmentId         any         //
	MediaType            any         //
	Name                 any         //
	FileUrl              any         //
	StoragePath          any         //
	MimeType             any         //
	Md5                  any         //
	Size                 any         //
	SortIndex            any         //
	Status               any         //
	CreatedBy            any         //
	UpdatedBy            any         //
	DeletedBy            any         //
	CreatedAt            *gtime.Time //
	UpdatedAt            *gtime.Time //
	DeletedAt            *gtime.Time //
	PerceptualHash       any         //
	Purpose              any         //
	PosterUrl            any         //
	TgFileId             any         //
	TgThumbFileId        any         //
	PosterStoragePath    any         //
	OriginalAttachmentId any         //
	OriginalFileUrl      any         //
	OriginalStoragePath  any         //
	EditedAttachmentId   any         //
	EditedFileUrl        any         //
	EditedStoragePath    any         //
	EditConfigJson       any         //
	EditStatus           any         //
	TgCacheAssetHash     any         //
	TgCacheStatus        any         //
}
