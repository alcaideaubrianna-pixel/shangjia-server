// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentMedia is the golang structure of table hg_content_media for DAO operations like Where/Data.
type ContentMedia struct {
	g.Meta              `orm:"table:hg_content_media, do:true"`
	Id                  any         // ID
	ProfileId           any         // 资料ID
	SourceAssetId       any         // FeiNiu资源ID
	DuplicateOfMediaId  any         // 重复媒体ID
	MediaType           any         // 媒体类型
	SortIndex           any         // 排序
	OriginalStoragePath any         // 原始存储路径
	DisplayStoragePath  any         // 展示存储路径
	PreviewStoragePath  any         // 预览存储路径
	BinaryMd5           any         // 文件MD5
	PerceptualHash      any         // 感知哈希
	Width               any         // 宽度
	Height              any         // 高度
	Duration            any         // 时长
	ProcessStatus       any         // 处理状态
	EncryptStatus       any         // 加密状态
	Status              any         // 状态
	CreatedAt           *gtime.Time // 创建时间
	UpdatedAt           *gtime.Time // 更新时间
	DeletedAt           *gtime.Time // 删除时间
}
