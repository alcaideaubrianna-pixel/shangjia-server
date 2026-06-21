// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentImportRun is the golang structure for table content_import_run.
type ContentImportRun struct {
	Id               int64       `json:"id"               orm:"id"                  description:"ID"`
	SourceName       string      `json:"sourceName"       orm:"source_name"         description:"来源名称"`
	TriggerType      string      `json:"triggerType"      orm:"trigger_type"        description:"触发方式"`
	BatchSize        int         `json:"batchSize"        orm:"batch_size"          description:"批量数量"`
	Scanned          int         `json:"scanned"          orm:"scanned"             description:"扫描数量"`
	Imported         int         `json:"imported"         orm:"imported"            description:"导入数量"`
	Duplicate        int         `json:"duplicate"        orm:"duplicate"           description:"重复数量"`
	MediaImported    int         `json:"mediaImported"    orm:"media_imported"      description:"媒体导入数量"`
	LastSourceNoteId int64       `json:"lastSourceNoteId" orm:"last_source_note_id" description:"最后来源笔记ID"`
	Status           string      `json:"status"           orm:"status"              description:"运行状态"`
	ErrorMessage     string      `json:"errorMessage"     orm:"error_message"       description:"错误信息"`
	StartedAt        *gtime.Time `json:"startedAt"        orm:"started_at"          description:"开始时间"`
	FinishedAt       *gtime.Time `json:"finishedAt"       orm:"finished_at"         description:"结束时间"`
	CostMs           int         `json:"costMs"           orm:"cost_ms"             description:"耗时毫秒"`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"          description:"创建时间"`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"          description:"更新时间"`
}
