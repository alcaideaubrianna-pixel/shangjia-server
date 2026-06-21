// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentImportRun is the golang structure of table hg_content_import_run for DAO operations like Where/Data.
type ContentImportRun struct {
	g.Meta           `orm:"table:hg_content_import_run, do:true"`
	Id               any         // ID
	SourceName       any         // 来源名称
	TriggerType      any         // 触发方式
	BatchSize        any         // 批量数量
	Scanned          any         // 扫描数量
	Imported         any         // 导入数量
	Duplicate        any         // 重复数量
	MediaImported    any         // 媒体导入数量
	LastSourceNoteId any         // 最后来源笔记ID
	Status           any         // 运行状态
	ErrorMessage     any         // 错误信息
	StartedAt        *gtime.Time // 开始时间
	FinishedAt       *gtime.Time // 结束时间
	CostMs           any         // 耗时毫秒
	CreatedAt        *gtime.Time // 创建时间
	UpdatedAt        *gtime.Time // 更新时间
}
