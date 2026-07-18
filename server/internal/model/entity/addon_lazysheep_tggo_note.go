// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoNote is the golang structure for table addon_lazysheep_tggo_note.
type AddonLazysheepTggoNote struct {
	Id               int64       `json:"id"               orm:"id"                 description:"主键"`
	BotId            int64       `json:"botId"            orm:"bot_id"             description:"机器人ID"`
	BindingId        int64       `json:"bindingId"        orm:"binding_id"         description:"绑定ID"`
	ContentId        int64       `json:"contentId"        orm:"content_id"         description:"内容ID"`
	UpId             int64       `json:"upId"             orm:"up_id"              description:"upId"`
	PairId           string      `json:"pairId"           orm:"pair_id"            description:"pairId"`
	ReceiverRoomId   int64       `json:"receiverRoomId"   orm:"receiver_room_id"   description:"房间ID"`
	RoomName         string      `json:"roomName"         orm:"room_name"          description:"房间名称"`
	Sender           string      `json:"sender"           orm:"sender"             description:"发送者"`
	SenderDno        string      `json:"senderDno"        orm:"sender_dno"         description:"发送设备"`
	SenderUser       *gjson.Json `json:"senderUser"       orm:"sender_user"        description:"发送用户"`
	RawPayload       *gjson.Json `json:"rawPayload"       orm:"raw_payload"        description:"原始消息"`
	NotePayload      *gjson.Json `json:"notePayload"      orm:"note_payload"       description:"笔记内容"`
	MessageType      string      `json:"messageType"      orm:"message_type"       description:"消息类型"`
	Code             string      `json:"code"             orm:"code"               description:"编号"`
	Title            string      `json:"title"            orm:"title"              description:"标题"`
	TextContent      string      `json:"textContent"      orm:"text_content"       description:"文本内容"`
	WorkflowStatus   int         `json:"workflowStatus"   orm:"workflow_status"    description:"流程状态"`
	ReviewMessageId  int64       `json:"reviewMessageId"  orm:"review_message_id"  description:"审核消息ID"`
	PublishMessageId int64       `json:"publishMessageId" orm:"publish_message_id" description:"推送消息ID"`
	ApprovedBy       int64       `json:"approvedBy"       orm:"approved_by"        description:"审核人"`
	PublishedBy      int64       `json:"publishedBy"      orm:"published_by"       description:"推送人"`
	ApprovedAt       *gtime.Time `json:"approvedAt"       orm:"approved_at"        description:"审核时间"`
	PublishedAt      *gtime.Time `json:"publishedAt"      orm:"published_at"       description:"推送时间"`
	LastError        string      `json:"lastError"        orm:"last_error"         description:"最后错误"`
	Sort             int         `json:"sort"             orm:"sort"               description:"排序"`
	Status           int         `json:"status"           orm:"status"             description:"状态"`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"         description:"创建时间"`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"         description:"更新时间"`
	DeletedAt        *gtime.Time `json:"deletedAt"        orm:"deleted_at"         description:"删除时间"`
}
