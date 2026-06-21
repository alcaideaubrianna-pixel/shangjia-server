// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentImportCheckpoint is the golang structure of table hg_content_import_checkpoint for DAO operations like Where/Data.
type ContentImportCheckpoint struct {
	g.Meta           `orm:"table:hg_content_import_checkpoint, do:true"`
	Id               any         // ID
	SourceName       any         // 来源名称
	LastSourceNoteId any         // 最后导入来源笔记ID
	LastSuccessAt    *gtime.Time // 最后成功时间
	LastError        any         // 最后错误
	CreatedAt        *gtime.Time // 创建时间
	UpdatedAt        *gtime.Time // 更新时间
}
