// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// AddonLazysheepTggoNote is the golang structure of table hg_addon_lazysheep_tggo_note for DAO operations like Where/Data.
type AddonLazysheepTggoNote struct {
	g.Meta           `orm:"table:hg_addon_lazysheep_tggo_note, do:true"`
	Id               any         // 主键
	BotId            any         // 机器人ID
	BindingId        any         // 绑定ID
	ContentId        any         // 内容ID
	UpId             any         // upId
	PairId           any         // pairId
	ReceiverRoomId   any         // 房间ID
	RoomName         any         // 房间名称
	Sender           any         // 发送者
	SenderDno        any         // 发送设备
	SenderUser       *gjson.Json // 发送用户
	RawPayload       *gjson.Json // 原始消息
	NotePayload      *gjson.Json // 笔记内容
	MessageType      any         // 消息类型
	Code             any         // 编号
	Title            any         // 标题
	TextContent      any         // 文本内容
	WorkflowStatus   any         // 流程状态
	ReviewMessageId  any         // 审核消息ID
	PublishMessageId any         // 推送消息ID
	ApprovedBy       any         // 审核人
	PublishedBy      any         // 推送人
	ApprovedAt       *gtime.Time // 审核时间
	PublishedAt      *gtime.Time // 推送时间
	LastError        any         // 最后错误
	Sort             any         // 排序
	Status           any         // 状态
	CreatedAt        *gtime.Time // 创建时间
	UpdatedAt        *gtime.Time // 更新时间
	DeletedAt        *gtime.Time // 删除时间
}
