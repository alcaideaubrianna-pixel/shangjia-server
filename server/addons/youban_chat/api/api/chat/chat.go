package chat

import (
	"hotgo/addons/youban_chat/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type StartReq struct {
	g.Meta `path:"/chat/start" method:"post" tags:"悦伴聊天" summary:"创建或打开聊天会话"`
	sysin.ChatStartInp
}

type StartRes struct {
	*sysin.ChatStartModel
}

type SendReq struct {
	g.Meta `path:"/chat/send" method:"post" tags:"悦伴聊天" summary:"发送聊天消息"`
	sysin.ChatSendInp
}

type SendRes struct {
	*sysin.ChatSendModel
}

type MessagesReq struct {
	g.Meta `path:"/chat/messages" method:"get" tags:"悦伴聊天" summary:"获取聊天消息"`
	sysin.ChatMessagesInp
}

type MessagesRes struct {
	*sysin.ChatMessagesModel
}

type ReadReq struct {
	g.Meta `path:"/chat/read" method:"post" tags:"悦伴聊天" summary:"标记聊天已读"`
	sysin.ChatReadInp
}

type ReadRes struct{}

type UploadReq struct {
	g.Meta `path:"/chat/upload" method:"post" tags:"悦伴聊天" summary:"发送聊天附件"`
	sysin.ChatUploadInp
}

type UploadRes struct {
	*sysin.ChatUploadModel
}

type UnreadReq struct {
	g.Meta `path:"/chat/unread" method:"get" tags:"悦伴聊天" summary:"获取聊天未读数"`
}

type UnreadRes struct {
	*sysin.ChatUnreadModel
}

type TelegramWebhookReq struct {
	g.Meta `path:"/telegram/webhook" method:"post" tags:"悦伴聊天" summary:"Telegram Webhook"`
	sysin.TelegramWebhookInp
}

type TelegramWebhookRes struct{}

type ListReq struct {
	g.Meta `path:"/chat/list" method:"get" tags:"悦伴聊天" summary:"聊天会话列表"`
	sysin.ChatConversationListInp
}

type ListRes struct {
	form.PageRes
	List []*sysin.ChatConversationListModel `json:"list" dc:"会话列表"`
}

type WidgetSessionReq struct {
	g.Meta `path:"/widget/session" method:"get" tags:"悦伴聊天" summary:"获取聊天会话配置"`
	sysin.ChatWidgetSessionInp
}

type WidgetSessionRes struct {
	*sysin.ChatWidgetSessionModel
}
