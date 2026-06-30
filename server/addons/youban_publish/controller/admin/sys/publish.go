package sys

import (
	"context"

	"hotgo/addons/youban_publish/api/admin/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

var Publish = cPublishServer{}
var PublishAdmin = cPublishAdmin{}

type cPublishServer struct{}
type cPublishAdmin struct{}

func (c *cPublishServer) TenantList(ctx context.Context, req *publish.TenantListReq) (res *publish.TenantListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTenantList(ctx, &req.TenantListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.TenantModel{}
	}
	res = new(publish.TenantListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) TenantSave(ctx context.Context, req *publish.TenantSaveReq) (res *publish.TenantSaveRes, err error) {
	saveRes, err := service.SysPublish().AdminTenantSave(ctx, &req.TenantSaveInp)
	if err != nil {
		return
	}
	res = &publish.TenantSaveRes{Password: ""}
	if saveRes != nil {
		res.Password = saveRes.Password
	}
	return
}

func (c *cPublishServer) TenantDelete(ctx context.Context, req *publish.TenantDeleteReq) (res *publish.TenantDeleteRes, err error) {
	err = service.SysPublish().AdminTenantDelete(ctx, &req.TenantDeleteInp)
	if err != nil {
		return
	}
	res = &publish.TenantDeleteRes{}
	return
}

func (c *cPublishServer) AccountList(ctx context.Context, req *publish.AccountListReq) (res *publish.AccountListRes, err error) {
	list, totalCount, err := service.SysPublish().ServerAccountList(ctx, &req.AccountListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AccountModel{}
	}
	res = new(publish.AccountListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) AccountSave(ctx context.Context, req *publish.AccountSaveReq) (res *publish.AccountSaveRes, err error) {
	saveRes, err := service.SysPublish().ServerAccountSave(ctx, &req.AccountSaveInp)
	if err != nil {
		return
	}
	res = &publish.AccountSaveRes{Password: ""}
	if saveRes != nil {
		res.Password = saveRes.Password
	}
	return
}

func (c *cPublishServer) AccountResetPassword(ctx context.Context, req *publish.AccountResetPasswordReq) (res *publish.AccountResetPasswordRes, err error) {
	saveRes, err := service.SysPublish().ServerAccountResetPassword(ctx, &req.AccountResetPasswordInp)
	if err != nil {
		return
	}
	res = &publish.AccountResetPasswordRes{Password: saveRes.Password}
	return
}

func (c *cPublishServer) AccountDelete(ctx context.Context, req *publish.AccountDeleteReq) (res *publish.AccountDeleteRes, err error) {
	err = service.SysPublish().ServerAccountDelete(ctx, &req.AccountDeleteInp)
	if err != nil {
		return
	}
	res = &publish.AccountDeleteRes{}
	return
}

func (c *cPublishAdmin) AccountList(ctx context.Context, req *publish.AccountListReq) (res *publish.AccountListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminAccountList(ctx, &req.AccountListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.AccountModel{}
	}
	res = new(publish.AccountListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) AccountSave(ctx context.Context, req *publish.AccountSaveReq) (res *publish.AccountSaveRes, err error) {
	saveRes, err := service.SysPublish().AdminAccountSave(ctx, &req.AccountSaveInp)
	if err != nil {
		return
	}
	res = &publish.AccountSaveRes{Password: ""}
	if saveRes != nil {
		res.Password = saveRes.Password
	}
	return
}

func (c *cPublishAdmin) AccountResetPassword(ctx context.Context, req *publish.AccountResetPasswordReq) (res *publish.AccountResetPasswordRes, err error) {
	saveRes, err := service.SysPublish().AdminAccountResetPassword(ctx, &req.AccountResetPasswordInp)
	if err != nil {
		return
	}
	res = &publish.AccountResetPasswordRes{Password: saveRes.Password}
	return
}

func (c *cPublishAdmin) AccountDelete(ctx context.Context, req *publish.AccountDeleteReq) (res *publish.AccountDeleteRes, err error) {
	err = service.SysPublish().AdminAccountDelete(ctx, &req.AccountDeleteInp)
	if err != nil {
		return
	}
	res = &publish.AccountDeleteRes{}
	return
}

func (c *cPublishAdmin) TaskList(ctx context.Context, req *publish.TaskListReq) (res *publish.TaskListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTaskList(ctx, &req.TaskListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.TaskModel{}
	}
	res = new(publish.TaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TaskSave(ctx context.Context, req *publish.TaskSaveReq) (res *publish.TaskSaveRes, err error) {
	id, err := service.SysPublish().AdminTaskSave(ctx, &req.TaskSaveInp)
	if err != nil {
		return
	}
	res = &publish.TaskSaveRes{Id: id}
	return
}

func (c *cPublishAdmin) TaskSubmit(ctx context.Context, req *publish.TaskSubmitReq) (res *publish.TaskSubmitRes, err error) {
	err = service.SysPublish().AdminTaskSubmit(ctx, &req.TaskSubmitInp)
	if err != nil {
		return
	}
	res = &publish.TaskSubmitRes{}
	return
}

func (c *cPublishAdmin) TaskCancel(ctx context.Context, req *publish.TaskCancelReq) (res *publish.TaskCancelRes, err error) {
	err = service.SysPublish().AdminTaskCancel(ctx, &req.TaskCancelInp)
	if err != nil {
		return
	}
	res = &publish.TaskCancelRes{}
	return
}

func (c *cPublishAdmin) MediaList(ctx context.Context, req *publish.MediaListReq) (res *publish.MediaListRes, err error) {
	list, err := service.SysPublish().AdminMediaList(ctx, &req.MediaListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.MediaModel{}
	}
	res = &publish.MediaListRes{List: list}
	return
}

func (c *cPublishAdmin) MediaDelete(ctx context.Context, req *publish.MediaDeleteReq) (res *publish.MediaDeleteRes, err error) {
	if err = service.SysPublish().AdminMediaDelete(ctx, &req.MediaDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.MediaDeleteRes{}
	return
}

func (c *cPublishAdmin) BotList(ctx context.Context, req *publish.BotListReq) (res *publish.BotListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminBotList(ctx, &req.BotListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.BotModel{}
	}
	res = new(publish.BotListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) BotSave(ctx context.Context, req *publish.BotSaveReq) (res *publish.BotSaveRes, err error) {
	if err = service.SysPublish().AdminBotSave(ctx, &req.BotSaveInp); err != nil {
		return nil, err
	}
	res = &publish.BotSaveRes{}
	return
}

func (c *cPublishAdmin) BotDelete(ctx context.Context, req *publish.BotDeleteReq) (res *publish.BotDeleteRes, err error) {
	if err = service.SysPublish().AdminBotDelete(ctx, &req.BotDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.BotDeleteRes{}
	return
}

func (c *cPublishAdmin) BotRefresh(ctx context.Context, req *publish.BotRefreshReq) (res *publish.BotRefreshRes, err error) {
	list, err := service.SysPublish().AdminBotRefresh(ctx, &req.BotRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.BotRefreshModel{}
	}
	res = &publish.BotRefreshRes{List: list}
	return
}

func (c *cPublishAdmin) TgAccountList(ctx context.Context, req *publish.TgAccountListReq) (res *publish.TgAccountListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTgAccountList(ctx, &req.TgAccountListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgAccountModel{}
	}
	res = new(publish.TgAccountListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TgAccountStartLogin(ctx context.Context, req *publish.TgAccountStartLoginReq) (res *publish.TgAccountStartLoginRes, err error) {
	item, err := service.SysPublish().AdminTgAccountStartLogin(ctx, &req.TgAccountStartLoginInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TgAccountStartLoginRes{TgAccountModel: item}
	return
}

func (c *cPublishAdmin) TgAccountLoginStatus(ctx context.Context, req *publish.TgAccountLoginStatusReq) (res *publish.TgAccountLoginStatusRes, err error) {
	item, err := service.SysPublish().AdminTgAccountLoginStatus(ctx, &req.TgAccountLoginStatusInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TgAccountLoginStatusRes{TgAccountModel: item}
	return
}

func (c *cPublishAdmin) TgAccountPassword(ctx context.Context, req *publish.TgAccountPasswordReq) (res *publish.TgAccountPasswordRes, err error) {
	item, err := service.SysPublish().AdminTgAccountPassword(ctx, &req.TgAccountPasswordInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TgAccountPasswordRes{TgAccountModel: item}
	return
}

func (c *cPublishAdmin) TgAccountDelete(ctx context.Context, req *publish.TgAccountDeleteReq) (res *publish.TgAccountDeleteRes, err error) {
	if err = service.SysPublish().AdminTgAccountDelete(ctx, &req.TgAccountDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.TgAccountDeleteRes{}
	return
}

func (c *cPublishAdmin) TgAccountRefresh(ctx context.Context, req *publish.TgAccountRefreshReq) (res *publish.TgAccountRefreshRes, err error) {
	list, err := service.SysPublish().AdminTgAccountRefresh(ctx, &req.TgAccountRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgAccountRefreshModel{}
	}
	res = &publish.TgAccountRefreshRes{List: list}
	return
}

func (c *cPublishAdmin) ChannelList(ctx context.Context, req *publish.ChannelListReq) (res *publish.ChannelListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminChannelList(ctx, &req.ChannelListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelModel{}
	}
	res = new(publish.ChannelListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) ChannelSave(ctx context.Context, req *publish.ChannelSaveReq) (res *publish.ChannelSaveRes, err error) {
	if err = service.SysPublish().AdminChannelSave(ctx, &req.ChannelSaveInp); err != nil {
		return nil, err
	}
	res = &publish.ChannelSaveRes{}
	return
}

func (c *cPublishAdmin) ChannelDelete(ctx context.Context, req *publish.ChannelDeleteReq) (res *publish.ChannelDeleteRes, err error) {
	if err = service.SysPublish().AdminChannelDelete(ctx, &req.ChannelDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.ChannelDeleteRes{}
	return
}

func (c *cPublishAdmin) ChannelBatchBots(ctx context.Context, req *publish.ChannelBatchBotsReq) (res *publish.ChannelBatchBotsRes, err error) {
	if err = service.SysPublish().AdminChannelBatchBots(ctx, &req.ChannelBatchBotsInp); err != nil {
		return nil, err
	}
	res = &publish.ChannelBatchBotsRes{}
	return
}

func (c *cPublishAdmin) ChannelRefresh(ctx context.Context, req *publish.ChannelRefreshReq) (res *publish.ChannelRefreshRes, err error) {
	list, err := service.SysPublish().AdminChannelRefresh(ctx, &req.ChannelRefreshInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ChannelRefreshModel{}
	}
	res = &publish.ChannelRefreshRes{List: list}
	return
}
