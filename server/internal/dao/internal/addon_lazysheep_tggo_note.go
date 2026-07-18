// ==========================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// AddonLazysheepTggoNoteDao is the data access object for the table hg_addon_lazysheep_tggo_note.
type AddonLazysheepTggoNoteDao struct {
	table    string                        // table is the underlying table name of the DAO.
	group    string                        // group is the database configuration group name of the current DAO.
	columns  AddonLazysheepTggoNoteColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler            // handlers for customized model modification.
}

// AddonLazysheepTggoNoteColumns defines and stores column names for the table hg_addon_lazysheep_tggo_note.
type AddonLazysheepTggoNoteColumns struct {
	Id               string // 主键
	BotId            string // 机器人ID
	BindingId        string // 绑定ID
	ContentId        string // 内容ID
	UpId             string // upId
	PairId           string // pairId
	ReceiverRoomId   string // 房间ID
	RoomName         string // 房间名称
	Sender           string // 发送者
	SenderDno        string // 发送设备
	SenderUser       string // 发送用户
	RawPayload       string // 原始消息
	NotePayload      string // 笔记内容
	MessageType      string // 消息类型
	Code             string // 编号
	Title            string // 标题
	TextContent      string // 文本内容
	WorkflowStatus   string // 流程状态
	ReviewMessageId  string // 审核消息ID
	PublishMessageId string // 推送消息ID
	ApprovedBy       string // 审核人
	PublishedBy      string // 推送人
	ApprovedAt       string // 审核时间
	PublishedAt      string // 推送时间
	LastError        string // 最后错误
	Sort             string // 排序
	Status           string // 状态
	CreatedAt        string // 创建时间
	UpdatedAt        string // 更新时间
	DeletedAt        string // 删除时间
}

// addonLazysheepTggoNoteColumns holds the columns for the table hg_addon_lazysheep_tggo_note.
var addonLazysheepTggoNoteColumns = AddonLazysheepTggoNoteColumns{
	Id:               "id",
	BotId:            "bot_id",
	BindingId:        "binding_id",
	ContentId:        "content_id",
	UpId:             "up_id",
	PairId:           "pair_id",
	ReceiverRoomId:   "receiver_room_id",
	RoomName:         "room_name",
	Sender:           "sender",
	SenderDno:        "sender_dno",
	SenderUser:       "sender_user",
	RawPayload:       "raw_payload",
	NotePayload:      "note_payload",
	MessageType:      "message_type",
	Code:             "code",
	Title:            "title",
	TextContent:      "text_content",
	WorkflowStatus:   "workflow_status",
	ReviewMessageId:  "review_message_id",
	PublishMessageId: "publish_message_id",
	ApprovedBy:       "approved_by",
	PublishedBy:      "published_by",
	ApprovedAt:       "approved_at",
	PublishedAt:      "published_at",
	LastError:        "last_error",
	Sort:             "sort",
	Status:           "status",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
	DeletedAt:        "deleted_at",
}

// NewAddonLazysheepTggoNoteDao creates and returns a new DAO object for table data access.
func NewAddonLazysheepTggoNoteDao(handlers ...gdb.ModelHandler) *AddonLazysheepTggoNoteDao {
	return &AddonLazysheepTggoNoteDao{
		group:    "default",
		table:    "hg_addon_lazysheep_tggo_note",
		columns:  addonLazysheepTggoNoteColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AddonLazysheepTggoNoteDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AddonLazysheepTggoNoteDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AddonLazysheepTggoNoteDao) Columns() AddonLazysheepTggoNoteColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AddonLazysheepTggoNoteDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AddonLazysheepTggoNoteDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

// Transaction wraps the transaction logic using function f.
// It rolls back the transaction and returns the error if function f returns a non-nil error.
// It commits the transaction and returns nil if function f returns nil.
//
// Note: Do not commit or roll back the transaction in function f,
// as it is automatically handled by this function.
func (dao *AddonLazysheepTggoNoteDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
