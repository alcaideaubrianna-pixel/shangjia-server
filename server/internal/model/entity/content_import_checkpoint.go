// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentImportCheckpoint is the golang structure for table content_import_checkpoint.
type ContentImportCheckpoint struct {
	Id               int64       `json:"id"               orm:"id"                  description:"ID"`
	SourceName       string      `json:"sourceName"       orm:"source_name"         description:"来源名称"`
	LastSourceNoteId int64       `json:"lastSourceNoteId" orm:"last_source_note_id" description:"最后导入来源笔记ID"`
	LastSuccessAt    *gtime.Time `json:"lastSuccessAt"    orm:"last_success_at"     description:"最后成功时间"`
	LastError        string      `json:"lastError"        orm:"last_error"          description:"最后错误"`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"          description:"创建时间"`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"          description:"更新时间"`
}
