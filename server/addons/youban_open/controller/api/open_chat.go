package api

import (
	"context"

	"hotgo/addons/youban_chat/model/input/sysin"
	chatservice "hotgo/addons/youban_chat/service"
	"hotgo/addons/youban_open/api/api/open"
	"hotgo/addons/youban_open/internal/opencontext"
)

var OpenChat = cOpenChat{}

type cOpenChat struct{}

func (c *cOpenChat) Session(ctx context.Context, req *open.ChatSessionReq) (*open.ChatSessionRes, error) {
	req.Visitor.AppId = opencontext.AppId(ctx)
	data, err := chatservice.SysChat().ExternalSession(ctx, &req.ExternalSessionInp)
	return &open.ChatSessionRes{ChatStartModel: data}, err
}
func (c *cOpenChat) Conversations(ctx context.Context, req *open.ChatConversationsReq) (*open.ChatConversationsRes, error) {
	req.Visitor.AppId = opencontext.AppId(ctx)
	data, err := chatservice.SysChat().ExternalConversations(ctx, &req.ExternalConversationsInp)
	return &open.ChatConversationsRes{ExternalConversationsModel: data}, err
}

func (c *cOpenChat) Pin(ctx context.Context, req *open.ChatPinReq) (*open.ChatPinRes, error) {
	req.Visitor.AppId = opencontext.AppId(ctx)
	err := chatservice.SysChat().ExternalPin(ctx, &req.ExternalConversationActionInp)
	return &open.ChatPinRes{}, err
}

func (c *cOpenChat) Delete(ctx context.Context, req *open.ChatDeleteReq) (*open.ChatDeleteRes, error) {
	req.Visitor.AppId = opencontext.AppId(ctx)
	err := chatservice.SysChat().ExternalDelete(ctx, &req.ExternalConversationActionInp)
	return &open.ChatDeleteRes{}, err
}

func (c *cOpenChat) Send(ctx context.Context, req *open.ChatSendReq) (*open.ChatSendRes, error) {
	req.Visitor.AppId = opencontext.AppId(ctx)
	data, err := chatservice.SysChat().ExternalSend(ctx, &req.ExternalMessageInp)
	return &open.ChatSendRes{ChatSendModel: data}, err
}

func (c *cOpenChat) Messages(ctx context.Context, req *open.ChatMessagesReq) (*open.ChatMessagesRes, error) {
	in := &sysin.ExternalMessagesInp{
		Visitor:        sysin.ExternalVisitorInp{AppId: opencontext.AppId(ctx), ExternalUserId: req.ExternalUserId, Name: req.Name, Email: req.Email, AvatarUrl: req.AvatarUrl},
		ConversationId: req.ConversationId,
		AfterId:        req.AfterId,
	}
	data, err := chatservice.SysChat().ExternalMessages(ctx, in)
	return &open.ChatMessagesRes{ChatMessagesModel: data}, err
}

func (c *cOpenChat) File(ctx context.Context, req *open.ChatFileReq) (*open.ChatFileRes, error) {
	req.Visitor.AppId = opencontext.AppId(ctx)
	data, err := chatservice.SysChat().ExternalFile(ctx, &req.ExternalFileInp)
	return &open.ChatFileRes{ChatUploadModel: data}, err
}

func (c *cOpenChat) Read(ctx context.Context, req *open.ChatReadReq) (*open.ChatReadRes, error) {
	req.Visitor.AppId = opencontext.AppId(ctx)
	if err := chatservice.SysChat().ExternalRead(ctx, &req.ExternalReadInp); err != nil {
		return nil, err
	}
	return &open.ChatReadRes{}, nil
}

func (c *cOpenChat) Unread(ctx context.Context, req *open.ChatUnreadReq) (*open.ChatUnreadRes, error) {
	in := &sysin.ExternalUnreadInp{Visitor: sysin.ExternalVisitorInp{AppId: opencontext.AppId(ctx), ExternalUserId: req.ExternalUserId, Name: req.Name, Email: req.Email, AvatarUrl: req.AvatarUrl}}
	data, err := chatservice.SysChat().ExternalUnread(ctx, in)
	return &open.ChatUnreadRes{ChatUnreadModel: data}, err
}

func (c *cOpenChat) Reaction(ctx context.Context, req *open.ChatReactionReq) (*open.ChatReactionRes, error) {
	req.Visitor.AppId = opencontext.AppId(ctx)
	if err := chatservice.SysChat().ExternalReaction(ctx, &req.ExternalReactionInp); err != nil {
		return nil, err
	}
	return &open.ChatReactionRes{}, nil
}

func (c *cOpenChat) AdminBots(ctx context.Context, req *open.ChatAdminBotsReq) (*open.ChatAdminBotsRes, error) {
	data, err := chatservice.SysChat().ExternalAdminBots(ctx, &sysin.ExternalAdminListInp{AppId: opencontext.AppId(ctx), Page: req.Page, PerPage: req.PageSize, Keyword: req.Keyword})
	return &open.ChatAdminBotsRes{ExternalAdminBotListModel: data}, err
}
func (c *cOpenChat) AdminSaveBot(ctx context.Context, req *open.ChatAdminSaveBotReq) (*open.ChatAdminSaveBotRes, error) {
	req.AppId = opencontext.AppId(ctx)
	if err := chatservice.SysChat().ExternalAdminSaveBot(ctx, &req.ExternalAdminBotSaveInp); err != nil {
		return nil, err
	}
	return &open.ChatAdminSaveBotRes{}, nil
}
func (c *cOpenChat) AdminConversations(ctx context.Context, req *open.ChatAdminConversationsReq) (*open.ChatAdminConversationsRes, error) {
	data, err := chatservice.SysChat().ExternalAdminConversations(ctx, &sysin.ExternalAdminConversationInp{AppId: opencontext.AppId(ctx), Page: req.Page, PerPage: req.PageSize})
	return &open.ChatAdminConversationsRes{ExternalAdminConversationListModel: data}, err
}
func (c *cOpenChat) AdminMessages(ctx context.Context, req *open.ChatAdminMessagesReq) (*open.ChatAdminMessagesRes, error) {
	data, err := chatservice.SysChat().ExternalAdminMessages(ctx, &sysin.ExternalAdminConversationInp{AppId: opencontext.AppId(ctx), ConversationId: req.Id})
	return &open.ChatAdminMessagesRes{ChatMessagesModel: data}, err
}
func (c *cOpenChat) AdminClear(ctx context.Context, req *open.ChatAdminClearReq) (*open.ChatAdminClearRes, error) {
	err := chatservice.SysChat().ExternalAdminClear(ctx, &sysin.ExternalAdminConversationInp{AppId: opencontext.AppId(ctx), ConversationId: req.Id})
	return &open.ChatAdminClearRes{}, err
}
func (c *cOpenChat) AdminDelete(ctx context.Context, req *open.ChatAdminDeleteReq) (*open.ChatAdminDeleteRes, error) {
	err := chatservice.SysChat().ExternalAdminDelete(ctx, &sysin.ExternalAdminConversationInp{AppId: opencontext.AppId(ctx), ConversationId: req.Id})
	return &open.ChatAdminDeleteRes{}, err
}
