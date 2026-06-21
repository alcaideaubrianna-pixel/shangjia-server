// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentSourceMap is the golang structure for table content_source_map.
type ContentSourceMap struct {
	Id              int64       `json:"id"              orm:"id"                description:"ID"`
	ProfileId       int64       `json:"profileId"       orm:"profile_id"        description:"资料ID"`
	SourceType      string      `json:"sourceType"      orm:"source_type"       description:"来源类型"`
	SourceKey       string      `json:"sourceKey"       orm:"source_key"        description:"来源唯一键"`
	SourceChannelId int64       `json:"sourceChannelId" orm:"source_channel_id" description:"来源频道ID"`
	SourceMessageId int64       `json:"sourceMessageId" orm:"source_message_id" description:"来源消息ID"`
	SourceGroupedId int64       `json:"sourceGroupedId" orm:"source_grouped_id" description:"来源媒体组ID"`
	SourceTextHash  string      `json:"sourceTextHash"  orm:"source_text_hash"  description:"来源文本哈希"`
	RawText         string      `json:"rawText"         orm:"raw_text"          description:"原始文本"`
	RawMessageJson  string      `json:"rawMessageJson"  orm:"raw_message_json"  description:"原始消息JSON"`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:"创建时间"`
}
