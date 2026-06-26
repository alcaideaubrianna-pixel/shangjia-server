package sys

import (
	"context"

	"hotgo/addons/youban_chat/api/admin/chat"
	"hotgo/addons/youban_chat/model/input/sysin"
	"hotgo/addons/youban_chat/service"
)

var Chat = cChat{}

type cChat struct{}

func (c *cChat) List(ctx context.Context, req *chat.ListReq) (res *chat.ListRes, err error) {
	list, totalCount, err := service.SysChat().AdminList(ctx, &req.AdminChatConversationListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AdminChatConversationListModel{}
	}
	res = new(chat.ListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cChat) View(ctx context.Context, req *chat.ViewReq) (res *chat.ViewRes, err error) {
	data, err := service.SysChat().AdminView(ctx, &req.AdminChatConversationViewInp)
	if err != nil {
		return
	}
	res = new(chat.ViewRes)
	res.AdminChatConversationViewModel = data
	return
}

func (c *cChat) Messages(ctx context.Context, req *chat.MessagesReq) (res *chat.MessagesRes, err error) {
	list, totalCount, err := service.SysChat().AdminMessages(ctx, &req.AdminChatMessageListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ChatMessageModel{}
	}
	res = new(chat.MessagesRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cChat) Clear(ctx context.Context, req *chat.ClearReq) (res *chat.ClearRes, err error) {
	err = service.SysChat().AdminClear(ctx, &req.AdminChatConversationClearInp)
	return
}

func (c *cChat) BotList(ctx context.Context, req *chat.BotListReq) (res *chat.BotListRes, err error) {
	list, totalCount, err := service.SysChat().AdminBotList(ctx, &req.AdminChatBotListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AdminChatBotModel{}
	}
	res = new(chat.BotListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cChat) SaveBot(ctx context.Context, req *chat.SaveBotReq) (res *chat.SaveBotRes, err error) {
	err = service.SysChat().AdminSaveBot(ctx, &req.AdminChatBotSaveInp)
	return
}

func (c *cChat) BindingList(ctx context.Context, req *chat.BindingListReq) (res *chat.BindingListRes, err error) {
	list, totalCount, err := service.SysChat().AdminBindingList(ctx, &req.AdminChatBindingListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AdminChatBindingModel{}
	}
	res = new(chat.BindingListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cChat) SaveBinding(ctx context.Context, req *chat.SaveBindingReq) (res *chat.SaveBindingRes, err error) {
	err = service.SysChat().AdminSaveBinding(ctx, &req.AdminChatBindingSaveInp)
	return
}

func (c *cChat) ChannelOptions(ctx context.Context, req *chat.ChannelOptionsReq) (res *chat.ChannelOptionsRes, err error) {
	list, err := service.SysChat().AdminChannelOptions(ctx)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AdminChatChannelOptionModel{}
	}
	res = new(chat.ChannelOptionsRes)
	res.List = list
	return
}

func (c *cChat) OperatorList(ctx context.Context, req *chat.OperatorListReq) (res *chat.OperatorListRes, err error) {
	list, totalCount, err := service.SysChat().AdminOperatorList(ctx, &req.AdminChatOperatorListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AdminChatOperatorModel{}
	}
	res = new(chat.OperatorListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cChat) SaveOperator(ctx context.Context, req *chat.SaveOperatorReq) (res *chat.SaveOperatorRes, err error) {
	err = service.SysChat().AdminSaveOperator(ctx, &req.AdminChatOperatorSaveInp)
	return
}

func (c *cChat) FeatureList(ctx context.Context, req *chat.FeatureListReq) (res *chat.FeatureListRes, err error) {
	list, totalCount, err := service.SysChat().AdminFeatureList(ctx, &req.AdminChatFeatureListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AdminChatFeatureModel{}
	}
	res = new(chat.FeatureListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cChat) SaveFeature(ctx context.Context, req *chat.SaveFeatureReq) (res *chat.SaveFeatureRes, err error) {
	err = service.SysChat().AdminSaveFeature(ctx, &req.AdminChatFeatureSaveInp)
	return
}
