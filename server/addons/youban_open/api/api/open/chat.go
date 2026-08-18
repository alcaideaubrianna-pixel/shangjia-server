package open

import (
	chatsysin "hotgo/addons/youban_chat/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

type ChatSessionReq struct {
	g.Meta `path:"/open/v1/chat/session" method:"post" tags:"开放聊天" summary:"创建或恢复客服会话"`
	chatsysin.ExternalSessionInp
}

type ChatSessionRes struct{ *chatsysin.ChatStartModel }
type ChatConversationsReq struct {
	g.Meta         `path:"/open/v1/chat/conversations" method:"get" tags:"开放聊天" summary:"获取客服会话列表"`
	ExternalUserId string `json:"externalUserId" v:"required#用户标识不能为空"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	AvatarUrl      string `json:"avatarUrl"`
}
type ChatConversationsRes struct {
	*chatsysin.ExternalConversationsModel
}

type ChatPinReq struct {
	g.Meta `path:"/open/v1/chat/conversations/pin" method:"post" tags:"开放聊天" summary:"置顶客服会话"`
	chatsysin.ExternalConversationActionInp
}
type ChatPinRes struct{}

type ChatDeleteReq struct {
	g.Meta `path:"/open/v1/chat/conversations/delete" method:"post" tags:"开放聊天" summary:"隐藏客服会话"`
	chatsysin.ExternalConversationActionInp
}
type ChatDeleteRes struct{}

type ChatSendReq struct {
	g.Meta `path:"/open/v1/chat/messages" method:"post" tags:"开放聊天" summary:"发送客服消息"`
	chatsysin.ExternalMessageInp
}

type ChatSendRes struct{ *chatsysin.ChatSendModel }

type ChatMessagesReq struct {
	g.Meta         `path:"/open/v1/chat/messages" method:"get" tags:"开放聊天" summary:"获取客服消息"`
	ExternalUserId string `json:"externalUserId" v:"required#用户标识不能为空"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	AvatarUrl      string `json:"avatarUrl"`
	ConversationId int64  `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空"`
	AfterId        int64  `json:"afterId"`
}

type ChatMessagesRes struct{ *chatsysin.ChatMessagesModel }

type ChatFileReq struct {
	g.Meta `path:"/open/v1/chat/files" method:"post" tags:"开放聊天" summary:"发送客服附件"`
	chatsysin.ExternalFileInp
}

type ChatFileRes struct{ *chatsysin.ChatUploadModel }

type ChatReadReq struct {
	g.Meta `path:"/open/v1/chat/read" method:"post" tags:"开放聊天" summary:"标记客服消息已读"`
	chatsysin.ExternalReadInp
}

type ChatReadRes struct{}

type ChatUnreadReq struct {
	g.Meta         `path:"/open/v1/chat/unread" method:"get" tags:"开放聊天" summary:"获取客服未读数"`
	ExternalUserId string `json:"externalUserId" v:"required#用户标识不能为空"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	AvatarUrl      string `json:"avatarUrl"`
}

type ChatUnreadRes struct{ *chatsysin.ChatUnreadModel }

type ChatReactionReq struct {
	g.Meta `path:"/open/v1/chat/reactions" method:"post" tags:"开放聊天" summary:"设置消息表情反应"`
	chatsysin.ExternalReactionInp
}

type ChatReactionRes struct{}

type ChatAdminBotsReq struct {
	g.Meta   `path:"/open/v1/chat/admin/bots" method:"get" tags:"开放聊天管理" summary:"租户Bot列表"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Keyword  string `json:"keyword"`
}
type ChatAdminBotsRes struct {
	*chatsysin.ExternalAdminBotListModel
}
type ChatAdminSaveBotReq struct {
	g.Meta `path:"/open/v1/chat/admin/bots/save" method:"post" tags:"开放聊天管理" summary:"注册租户Bot"`
	chatsysin.ExternalAdminBotSaveInp
}
type ChatAdminSaveBotRes struct{}
type ChatAdminDeleteBotReq struct {
	g.Meta `path:"/open/v1/chat/admin/bots/{id}" method:"delete" tags:"开放聊天管理" summary:"删除租户Bot"`
	Id     int64 `json:"id"`
}
type ChatAdminDeleteBotRes struct{}
type ChatAdminRotateBotBindingCodeReq struct {
	g.Meta `path:"/open/v1/chat/admin/bots/{id}/binding-code/rotate" method:"post" tags:"开放聊天管理" summary:"刷新租户Bot绑定码"`
	Id     int64 `json:"id"`
}
type ChatAdminRotateBotBindingCodeRes struct{}
type ChatAdminConversationsReq struct {
	g.Meta   `path:"/open/v1/chat/admin/conversations" method:"get" tags:"开放聊天管理" summary:"租户会话列表"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}
type ChatAdminConversationsRes struct {
	*chatsysin.ExternalAdminConversationListModel
}
type ChatAdminMessagesReq struct {
	g.Meta `path:"/open/v1/chat/admin/conversations/{id}/messages" method:"get" tags:"开放聊天管理" summary:"租户聊天记录"`
	Id     int64 `json:"id"`
}
type ChatAdminMessagesRes struct{ *chatsysin.ChatMessagesModel }
type ChatAdminClearReq struct {
	g.Meta `path:"/open/v1/chat/admin/conversations/{id}/clear" method:"post" tags:"开放聊天管理" summary:"清空租户聊天记录"`
	Id     int64 `json:"id"`
}
type ChatAdminClearRes struct{}
type ChatAdminDeleteReq struct {
	g.Meta `path:"/open/v1/chat/admin/conversations/{id}" method:"delete" tags:"开放聊天管理" summary:"删除租户会话"`
	Id     int64 `json:"id"`
}
type ChatAdminDeleteRes struct{}
