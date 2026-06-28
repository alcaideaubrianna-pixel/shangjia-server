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
	g.Meta       `orm:"table:hg_youban_publish_media, do:true"`
	Id           any         // 主键
	MerchantId   any         // 商家ID
	AccountId    any         // 账号ID
	TaskId       any         // 任务ID
	ProfileId    any         // 资料ID
	AttachmentId any         // HotGo附件ID
	MediaType    any         // 媒体类型
	Name         any         // 文件名
	FileUrl      any         // 访问地址
	StoragePath  any         // 存储路径
	MimeType     any         // MIME
	Md5          any         // MD5
	Size         any         // 大小
	SortIndex    any         // 排序
	Status       any         // 状态
	CreatedBy    any         // 创建人
	UpdatedBy    any         // 更新人
	DeletedBy    any         // 删除人
	CreatedAt    *gtime.Time // 创建时间
	UpdatedAt    *gtime.Time // 更新时间
	DeletedAt    *gtime.Time // 删除时间
}
