// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentSourceMap is the golang structure of table hg_content_source_map for DAO operations like Where/Data.
type ContentSourceMap struct {
	g.Meta          `orm:"table:hg_content_source_map, do:true"`
	Id              any         // ID
	ProfileId       any         // 资料ID
	SourceType      any         // 来源类型
	SourceKey       any         // 来源唯一键
	SourceChannelId any         // 来源频道ID
	SourceMessageId any         // 来源消息ID
	SourceGroupedId any         // 来源媒体组ID
	SourceTextHash  any         // 来源文本哈希
	RawText         any         // 原始文本
	RawMessageJson  any         // 原始消息JSON
	CreatedAt       *gtime.Time // 创建时间
}
