package service

import (
	"context"
	"hotgo/addons/youban_chat/model/input/sysin"

	"github.com/gogf/gf/v2/net/ghttp"
)

type ISysChat interface {
	Start(ctx context.Context, in *sysin.ChatStartInp) (res *sysin.ChatStartModel, err error)
	Send(ctx context.Context, in *sysin.ChatSendInp) (res *sysin.ChatSendModel, err error)
	Messages(ctx context.Context, in *sysin.ChatMessagesInp) (res *sysin.ChatMessagesModel, err error)
	Pin(ctx context.Context, in *sysin.ChatConversationPinInp) (err error)
	Clear(ctx context.Context, in *sysin.ChatConversationClearInp) (err error)
	Read(ctx context.Context, in *sysin.ChatReadInp) (err error)
	Upload(ctx context.Context, in *sysin.ChatUploadInp, file *ghttp.UploadFile) (res *sysin.ChatUploadModel, err error)
	Unread(ctx context.Context) (res *sysin.ChatUnreadModel, err error)
	ExternalSession(ctx context.Context, in *sysin.ExternalSessionInp) (res *sysin.ChatStartModel, err error)
	ExternalConversations(ctx context.Context, in *sysin.ExternalConversationsInp) (res *sysin.ExternalConversationsModel, err error)
	ExternalPin(ctx context.Context, in *sysin.ExternalConversationActionInp) (err error)
	ExternalDelete(ctx context.Context, in *sysin.ExternalConversationActionInp) (err error)
	ExternalAdminBots(ctx context.Context, in *sysin.ExternalAdminListInp) (res *sysin.ExternalAdminBotListModel, err error)
	ExternalAdminSaveBot(ctx context.Context, in *sysin.ExternalAdminBotSaveInp) (err error)
	ExternalAdminDeleteBot(ctx context.Context, in *sysin.ExternalAdminBotActionInp) (err error)
	ExternalAdminRotateBotBindingCode(ctx context.Context, in *sysin.ExternalAdminBotActionInp) (err error)
	ExternalAdminCheckBot(ctx context.Context, in *sysin.ExternalAdminBotCheckInp) (res *sysin.ExternalAdminBotCheckModel, err error)
	ExternalAdminConversations(ctx context.Context, in *sysin.ExternalAdminConversationInp) (res *sysin.ExternalAdminConversationListModel, err error)
	ExternalAdminMessages(ctx context.Context, in *sysin.ExternalAdminConversationInp) (res *sysin.ChatMessagesModel, err error)
	ExternalAdminClear(ctx context.Context, in *sysin.ExternalAdminConversationInp) (err error)
	ExternalAdminDelete(ctx context.Context, in *sysin.ExternalAdminConversationInp) (err error)
	ExternalSend(ctx context.Context, in *sysin.ExternalMessageInp) (res *sysin.ChatSendModel, err error)
	ExternalMessages(ctx context.Context, in *sysin.ExternalMessagesInp) (res *sysin.ChatMessagesModel, err error)
	ExternalFile(ctx context.Context, in *sysin.ExternalFileInp) (res *sysin.ChatUploadModel, err error)
	ExternalRead(ctx context.Context, in *sysin.ExternalReadInp) (err error)
	ExternalUnread(ctx context.Context, in *sysin.ExternalUnreadInp) (res *sysin.ChatUnreadModel, err error)
	ExternalReaction(ctx context.Context, in *sysin.ExternalReactionInp) (err error)
	TelegramWebhook(ctx context.Context, in *sysin.TelegramWebhookInp) (err error)
	List(ctx context.Context, in *sysin.ChatConversationListInp) (list []*sysin.ChatConversationListModel, totalCount int, err error)
	WidgetSession(ctx context.Context, in *sysin.ChatWidgetSessionInp) (res *sysin.ChatWidgetSessionModel, err error)
	AdminList(ctx context.Context, in *sysin.AdminChatConversationListInp) (list []*sysin.AdminChatConversationListModel, totalCount int, err error)
	AdminView(ctx context.Context, in *sysin.AdminChatConversationViewInp) (res *sysin.AdminChatConversationViewModel, err error)
	AdminMessages(ctx context.Context, in *sysin.AdminChatMessageListInp) (list []*sysin.ChatMessageModel, totalCount int, err error)
	AdminClear(ctx context.Context, in *sysin.AdminChatConversationClearInp) (err error)
	AdminBotList(ctx context.Context, in *sysin.AdminChatBotListInp) (list []*sysin.AdminChatBotModel, totalCount int, err error)
	AdminSaveBot(ctx context.Context, in *sysin.AdminChatBotSaveInp) (err error)
	AdminBindingList(ctx context.Context, in *sysin.AdminChatBindingListInp) (list []*sysin.AdminChatBindingModel, totalCount int, err error)
	AdminSaveBinding(ctx context.Context, in *sysin.AdminChatBindingSaveInp) (err error)
	AdminChannelOptions(ctx context.Context) (list []*sysin.AdminChatChannelOptionModel, err error)
	AdminOperatorList(ctx context.Context, in *sysin.AdminChatOperatorListInp) (list []*sysin.AdminChatOperatorModel, totalCount int, err error)
	AdminSaveOperator(ctx context.Context, in *sysin.AdminChatOperatorSaveInp) (err error)
	AdminFeatureList(ctx context.Context, in *sysin.AdminChatFeatureListInp) (list []*sysin.AdminChatFeatureModel, totalCount int, err error)
	AdminSaveFeature(ctx context.Context, in *sysin.AdminChatFeatureSaveInp) (err error)
}

var localSysChat ISysChat

func SysChat() ISysChat {
	if localSysChat == nil {
		panic("implement not found for interface ISysChat, forgot register?")
	}
	return localSysChat
}

func RegisterSysChat(i ISysChat) {
	localSysChat = i
}
