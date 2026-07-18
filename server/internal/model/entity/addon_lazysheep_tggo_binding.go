// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoBinding is the golang structure for table addon_lazysheep_tggo_binding.
type AddonLazysheepTggoBinding struct {
	Id              int64       `json:"id"              orm:"id"               description:"主键"`
	BindingKey      string      `json:"bindingKey"      orm:"binding_key"      description:"绑定标识"`
	BotId           int64       `json:"botId"           orm:"bot_id"           description:"机器人ID"`
	BotKey          string      `json:"botKey"          orm:"bot_key"          description:"机器人标识"`
	SourceUrl       string      `json:"sourceUrl"       orm:"source_url"       description:"BangChat 链接"`
	SourceToken     string      `json:"sourceToken"     orm:"source_token"     description:"BangChat token"`
	SourceRoomId    int64       `json:"sourceRoomId"    orm:"source_room_id"   description:"来源房间ID"`
	SourcePairId    string      `json:"sourcePairId"    orm:"source_pair_id"   description:"来源 pairId"`
	ReviewChatId    int64       `json:"reviewChatId"    orm:"review_chat_id"   description:"审核群ID"`
	PublishChatId   int64       `json:"publishChatId"   orm:"publish_chat_id"  description:"推送频道ID"`
	AutoPush        int         `json:"autoPush"        orm:"auto_push"        description:"自动推送"`
	ReviewEnabled   int         `json:"reviewEnabled"   orm:"review_enabled"   description:"审核开关"`
	PublishEnabled  int         `json:"publishEnabled"  orm:"publish_enabled"  description:"推送开关"`
	VerifyEnabled   int         `json:"verifyEnabled"   orm:"verify_enabled"   description:"验证按钮开关"`
	LocationEnabled int         `json:"locationEnabled" orm:"location_enabled" description:"位置按钮开关"`
	PluginSettings  string      `json:"pluginSettings"  orm:"plugin_settings"   description:"插件状态"`
	LastPullId      int64       `json:"lastPullId"      orm:"last_pull_id"     description:"最后拉取ID"`
	LastCursor      string      `json:"lastCursor"      orm:"last_cursor"      description:"最后游标"`
	Status          int         `json:"status"          orm:"status"           description:"状态"`
	CreatedBy       int64       `json:"createdBy"       orm:"created_by"       description:"创建者"`
	UpdatedBy       int64       `json:"updatedBy"       orm:"updated_by"       description:"更新者"`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"       description:"创建时间"`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"       description:"更新时间"`
}
