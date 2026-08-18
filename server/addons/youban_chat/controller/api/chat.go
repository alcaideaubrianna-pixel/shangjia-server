package api

import (
	"context"

	"hotgo/addons/youban_chat/api/api/chat"
	"hotgo/addons/youban_chat/model/input/sysin"
	"hotgo/addons/youban_chat/service"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

var Chat = cChat{}

type cChat struct{}

func (c *cChat) Start(ctx context.Context, req *chat.StartReq) (res *chat.StartRes, err error) {
	data, err := service.SysChat().Start(ctx, &req.ChatStartInp)
	if err != nil {
		return
	}
	res = &chat.StartRes{ChatStartModel: data}
	return
}

func (c *cChat) Send(ctx context.Context, req *chat.SendReq) (res *chat.SendRes, err error) {
	data, err := service.SysChat().Send(ctx, &req.ChatSendInp)
	if err != nil {
		return
	}
	res = &chat.SendRes{ChatSendModel: data}
	return
}

func (c *cChat) Messages(ctx context.Context, req *chat.MessagesReq) (res *chat.MessagesRes, err error) {
	data, err := service.SysChat().Messages(ctx, &req.ChatMessagesInp)
	if err != nil {
		return
	}
	res = &chat.MessagesRes{ChatMessagesModel: data}
	return
}

func (c *cChat) Pin(ctx context.Context, req *chat.PinReq) (res *chat.PinRes, err error) {
	err = service.SysChat().Pin(ctx, &req.ChatConversationPinInp)
	if err != nil {
		return
	}
	res = &chat.PinRes{}
	return
}

func (c *cChat) Clear(ctx context.Context, req *chat.ClearReq) (res *chat.ClearRes, err error) {
	err = service.SysChat().Clear(ctx, &req.ChatConversationClearInp)
	if err != nil {
		return
	}
	res = &chat.ClearRes{}
	return
}

func (c *cChat) Read(ctx context.Context, req *chat.ReadReq) (res *chat.ReadRes, err error) {
	err = service.SysChat().Read(ctx, &req.ChatReadInp)
	if err != nil {
		return
	}
	res = &chat.ReadRes{}
	return
}

func (c *cChat) Upload(ctx context.Context, req *chat.UploadReq) (res *chat.UploadRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("file")
	if file == nil {
		err = gerror.New("没有找到上传的文件")
		return
	}
	data, err := service.SysChat().Upload(ctx, &req.ChatUploadInp, file)
	if err != nil {
		return
	}
	res = &chat.UploadRes{ChatUploadModel: data}
	return
}

func (c *cChat) Unread(ctx context.Context, req *chat.UnreadReq) (res *chat.UnreadRes, err error) {
	data, err := service.SysChat().Unread(ctx)
	if err != nil {
		return
	}
	res = &chat.UnreadRes{ChatUnreadModel: data}
	return
}

func (c *cChat) List(ctx context.Context, req *chat.ListReq) (res *chat.ListRes, err error) {
	list, totalCount, err := service.SysChat().List(ctx, &req.ChatConversationListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ChatConversationListModel{}
	}
	res = new(chat.ListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cChat) WidgetSession(ctx context.Context, req *chat.WidgetSessionReq) (res *chat.WidgetSessionRes, err error) {
	data, err := service.SysChat().WidgetSession(ctx, &req.ChatWidgetSessionInp)
	if err != nil {
		return
	}
	res = &chat.WidgetSessionRes{ChatWidgetSessionModel: data}
	return
}
