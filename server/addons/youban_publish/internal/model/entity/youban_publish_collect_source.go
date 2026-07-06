// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectSource is the golang structure for table youban_publish_collect_source.
type YoubanPublishCollectSource struct {
	Id              uint64      `json:"id"              orm:"id"                description:"主键"`
	TenantId        int64       `json:"tenantId"        orm:"tenant_id"         description:"租户ID"`
	AccountId       int64       `json:"accountId"       orm:"account_id"        description:"所属账号ID"`
	SourceType      string      `json:"sourceType"      orm:"source_type"       description:"来源类型"`
	Title           string      `json:"title"           orm:"title"             description:"采集源名称"`
	SourceChatId    string      `json:"sourceChatId"    orm:"source_chat_id"    description:"来源频道/群聊ID"`
	SourceUsername  string      `json:"sourceUsername"  orm:"source_username"   description:"来源用户名"`
	TgAccountId     int64       `json:"tgAccountId"     orm:"tg_account_id"     description:"协议号ID"`
	BotId           int64       `json:"botId"           orm:"bot_id"            description:"机器人ID"`
	FollowAccountId int64       `json:"followAccountId" orm:"follow_account_id" description:"关注账号ID"`
	CollectEnabled  int         `json:"collectEnabled"  orm:"collect_enabled"   description:"是否开启采集"`
	Status          int         `json:"status"          orm:"status"            description:"状态"`
	EventTotal      int64       `json:"eventTotal"      orm:"event_total"       description:"事件总数"`
	SuccessTotal    int64       `json:"successTotal"    orm:"success_total"     description:"成功数"`
	FailedTotal     int64       `json:"failedTotal"     orm:"failed_total"      description:"失败数"`
	LastEventAt     *gtime.Time `json:"lastEventAt"     orm:"last_event_at"     description:"最后事件时间"`
	Remark          string      `json:"remark"          orm:"remark"            description:"备注"`
	CreatedBy       int64       `json:"createdBy"       orm:"created_by"        description:"创建人"`
	UpdatedBy       int64       `json:"updatedBy"       orm:"updated_by"        description:"更新人"`
	DeletedBy       int64       `json:"deletedBy"       orm:"deleted_by"        description:"删除人"`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        description:"更新时间"`
	DeletedAt       *gtime.Time `json:"deletedAt"       orm:"deleted_at"        description:"删除时间"`
}
