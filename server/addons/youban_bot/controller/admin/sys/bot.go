package sys

import (
	"context"

	"hotgo/addons/youban_bot/api/admin/bot"
	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/addons/youban_bot/service"
)

var Bot = cBot{}

type cBot struct{}

func (c *cBot) BotList(ctx context.Context, req *bot.BotListReq) (res *bot.BotListRes, err error) {
	list, totalCount, err := service.SysBot().AdminBotList(ctx, &req.BotListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.BotModel{}
	}
	res = new(bot.BotListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cBot) BotSave(ctx context.Context, req *bot.BotSaveReq) (res *bot.BotSaveRes, err error) {
	if err = service.SysBot().AdminBotSave(ctx, &req.BotSaveInp); err != nil {
		return
	}
	res = &bot.BotSaveRes{}
	return
}

func (c *cBot) BotDelete(ctx context.Context, req *bot.BotDeleteReq) (res *bot.BotDeleteRes, err error) {
	if err = service.SysBot().AdminBotDelete(ctx, &req.BotDeleteInp); err != nil {
		return
	}
	res = &bot.BotDeleteRes{}
	return
}

func (c *cBot) BotRefresh(ctx context.Context, req *bot.BotRefreshReq) (res *bot.BotRefreshRes, err error) {
	list, err := service.SysBot().AdminBotRefresh(ctx, &req.BotRefreshInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.BotRefreshModel{}
	}
	res = &bot.BotRefreshRes{List: list}
	return
}

func (c *cBot) BotRestart(ctx context.Context, req *bot.BotRestartReq) (res *bot.BotRestartRes, err error) {
	list, err := service.SysBot().AdminBotRestart(ctx, &req.BotRefreshInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.BotRefreshModel{}
	}
	res = &bot.BotRestartRes{List: list}
	return
}

func (c *cBot) FeatureList(ctx context.Context, req *bot.FeatureListReq) (res *bot.FeatureListRes, err error) {
	list, totalCount, err := service.SysBot().AdminFeatureList(ctx, &req.FeatureListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.FeatureModel{}
	}
	res = new(bot.FeatureListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cBot) FeatureSave(ctx context.Context, req *bot.FeatureSaveReq) (res *bot.FeatureSaveRes, err error) {
	if err = service.SysBot().AdminFeatureSave(ctx, &req.FeatureSaveInp); err != nil {
		return
	}
	res = &bot.FeatureSaveRes{}
	return
}

func (c *cBot) UserList(ctx context.Context, req *bot.UserListReq) (res *bot.UserListRes, err error) {
	list, totalCount, err := service.SysBot().AdminUserList(ctx, &req.UserListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.UserModel{}
	}
	res = new(bot.UserListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cBot) AccountBindList(ctx context.Context, req *bot.AccountBindListReq) (res *bot.AccountBindListRes, err error) {
	list, totalCount, err := service.SysBot().AdminAccountBindList(ctx, &req.AccountBindListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AccountBindModel{}
	}
	res = new(bot.AccountBindListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cBot) AccountBindUnbind(ctx context.Context, req *bot.AccountBindUnbindReq) (res *bot.AccountBindUnbindRes, err error) {
	if err = service.SysBot().AdminAccountBindUnbind(ctx, &req.AccountBindUnbindInp); err != nil {
		return
	}
	res = &bot.AccountBindUnbindRes{}
	return
}

func (c *cBot) MessageList(ctx context.Context, req *bot.MessageListReq) (res *bot.MessageListRes, err error) {
	list, totalCount, err := service.SysBot().AdminMessageList(ctx, &req.MessageListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.MessageModel{}
	}
	res = new(bot.MessageListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cBot) BotChannelCacheList(ctx context.Context, req *bot.BotChannelCacheListReq) (res *bot.BotChannelCacheListRes, err error) {
	list, totalCount, err := service.SysBot().AdminBotChannelCacheList(ctx, &req.BotChannelCacheListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.BotChannelCacheModel{}
	}
	res = new(bot.BotChannelCacheListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cBot) UserSwitchSuperAdmin(ctx context.Context, req *bot.UserSwitchSuperAdminReq) (res *bot.UserSwitchSuperAdminRes, err error) {
	if err = service.SysBot().AdminUserSwitchSuperAdmin(ctx, &req.UserSwitchSuperAdminInp); err != nil {
		return
	}
	res = &bot.UserSwitchSuperAdminRes{}
	return
}

func (c *cBot) SendMessage(ctx context.Context, req *bot.SendMessageReq) (res *bot.SendMessageRes, err error) {
	if err = service.SysBot().AdminSendMessage(ctx, &req.SendMessageInp); err != nil {
		return
	}
	res = &bot.SendMessageRes{}
	return
}

func (c *cBot) BroadcastCreate(ctx context.Context, req *bot.BroadcastCreateReq) (res *bot.BroadcastCreateRes, err error) {
	task, err := service.SysBot().AdminBroadcastCreate(ctx, &req.BroadcastCreateInp)
	if err != nil {
		return nil, err
	}
	return &bot.BroadcastCreateRes{BroadcastTaskModel: task}, nil
}

func (c *cBot) BroadcastTask(ctx context.Context, req *bot.BroadcastTaskReq) (res *bot.BroadcastTaskRes, err error) {
	task, err := service.SysBot().AdminBroadcastTask(ctx, &req.BroadcastTaskInp)
	if err != nil {
		return nil, err
	}
	return &bot.BroadcastTaskRes{BroadcastTaskModel: task}, nil
}
