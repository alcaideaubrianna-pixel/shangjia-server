// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishChannel is the golang structure for table youban_publish_channel.
type YoubanPublishChannel struct {
	Id                  uint64      `json:"id"                  orm:"id"                    description:"主键"`
	TenantId            int64       `json:"tenantId"            orm:"tenant_id"             description:"租户ID"`
	MerchantId          int64       `json:"merchantId"          orm:"merchant_id"           description:"兼容旧版本商家ID"`
	TgAccountId         int64       `json:"tgAccountId"         orm:"tg_account_id"         description:"TG账号ID"`
	ChannelTitle        string      `json:"channelTitle"        orm:"channel_title"         description:"频道名称"`
	ChannelUsername     string      `json:"channelUsername"     orm:"channel_username"      description:"频道用户名"`
	TargetChatId        string      `json:"targetChatId"        orm:"target_chat_id"        description:"目标Chat ID"`
	PublishDirection    string      `json:"publishDirection"    orm:"publish_direction"     description:"上架/下架频道"`
	CyclePublishEnabled int         `json:"cyclePublishEnabled" orm:"cycle_publish_enabled" description:"是否循环上架"`
	CyclePublishDays    int         `json:"cyclePublishDays"    orm:"cycle_publish_days"    description:"循环上架天数"`
	CyclePublishTime    string      `json:"cyclePublishTime"    orm:"cycle_publish_time"    description:"循环上架时间"`
	IsDefaultSelected   int         `json:"isDefaultSelected"   orm:"is_default_selected"   description:"是否默认选中"`
	BotIdJson           string      `json:"botIdJson"           orm:"bot_id_json"           description:"绑定Bot ID JSON"`
	Remark              string      `json:"remark"              orm:"remark"                description:"备注"`
	Status              int         `json:"status"              orm:"status"                description:"状态"`
	LastRefreshStatus   string      `json:"lastRefreshStatus"   orm:"last_refresh_status"   description:"最近刷新状态"`
	LastRefreshMessage  string      `json:"lastRefreshMessage"  orm:"last_refresh_message"  description:"最近刷新信息"`
	LastRefreshAt       *gtime.Time `json:"lastRefreshAt"       orm:"last_refresh_at"       description:"最近刷新时间"`
	CreatedBy           int64       `json:"createdBy"           orm:"created_by"            description:"创建人"`
	UpdatedBy           int64       `json:"updatedBy"           orm:"updated_by"            description:"更新人"`
	DeletedBy           int64       `json:"deletedBy"           orm:"deleted_by"            description:"删除人"`
	CreatedAt           *gtime.Time `json:"createdAt"           orm:"created_at"            description:"创建时间"`
	UpdatedAt           *gtime.Time `json:"updatedAt"           orm:"updated_at"            description:"更新时间"`
	DeletedAt           *gtime.Time `json:"deletedAt"           orm:"deleted_at"            description:"删除时间"`
}
