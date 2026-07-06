// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectEvent is the golang structure of table hg_youban_publish_collect_event for DAO operations like Where/Data.
type YoubanPublishCollectEvent struct {
	g.Meta          `orm:"table:hg_youban_publish_collect_event, do:true"`
	Id              any         // 主键
	TenantId        any         // 租户ID
	AccountId       any         // 所属账号ID
	SourceId        any         // 采集源ID
	SourceType      any         // 来源类型
	BotId           any         // 机器人ID
	TgAccountId     any         // 协议号ID
	SourceChatId    any         // 来源频道/群聊ID
	SourceMessageId any         // 来源消息ID
	SourceGroupedId any         // 媒体组ID
	SourceUniqueKey any         // 来源唯一键
	RawText         any         // 原始文本
	MediaCount      any         // 媒体数量
	MediaJson       any         // 媒体JSON
	TextHash        any         // 文本哈希
	DedupeKey       any         // 去重键
	Status          any         // 状态
	ErrorMessage    any         // 错误信息
	ReceivedAt      *gtime.Time // 接收时间
	ProcessedAt     *gtime.Time // 处理时间
	CreatedAt       *gtime.Time // 创建时间
	UpdatedAt       *gtime.Time // 更新时间
}
