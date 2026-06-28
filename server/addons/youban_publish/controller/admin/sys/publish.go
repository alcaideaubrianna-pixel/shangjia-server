package sys

import (
	"context"

	"hotgo/addons/youban_publish/api/admin/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

var Publish = cPublish{}

type cPublish struct{}

func (c *cPublish) MerchantList(ctx context.Context, req *publish.MerchantListReq) (res *publish.MerchantListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminMerchantList(ctx, &req.MerchantListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.MerchantModel{}
	}
	res = new(publish.MerchantListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) MerchantSave(ctx context.Context, req *publish.MerchantSaveReq) (res *publish.MerchantSaveRes, err error) {
	err = service.SysPublish().AdminMerchantSave(ctx, &req.MerchantSaveInp)
	if err != nil {
		return
	}
	res = &publish.MerchantSaveRes{}
	return
}

func (c *cPublish) MerchantDelete(ctx context.Context, req *publish.MerchantDeleteReq) (res *publish.MerchantDeleteRes, err error) {
	err = service.SysPublish().AdminMerchantDelete(ctx, &req.MerchantDeleteInp)
	if err != nil {
		return
	}
	res = &publish.MerchantDeleteRes{}
	return
}

func (c *cPublish) AccountList(ctx context.Context, req *publish.AccountListReq) (res *publish.AccountListRes, err error) {
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

func (c *cPublish) AccountSave(ctx context.Context, req *publish.AccountSaveReq) (res *publish.AccountSaveRes, err error) {
	err = service.SysPublish().AdminAccountSave(ctx, &req.AccountSaveInp)
	if err != nil {
		return
	}
	res = &publish.AccountSaveRes{}
	return
}

func (c *cPublish) AccountDelete(ctx context.Context, req *publish.AccountDeleteReq) (res *publish.AccountDeleteRes, err error) {
	err = service.SysPublish().AdminAccountDelete(ctx, &req.AccountDeleteInp)
	if err != nil {
		return
	}
	res = &publish.AccountDeleteRes{}
	return
}

func (c *cPublish) TaskList(ctx context.Context, req *publish.TaskListReq) (res *publish.TaskListRes, err error) {
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

func (c *cPublish) TaskSave(ctx context.Context, req *publish.TaskSaveReq) (res *publish.TaskSaveRes, err error) {
	id, err := service.SysPublish().AdminTaskSave(ctx, &req.TaskSaveInp)
	if err != nil {
		return
	}
	res = &publish.TaskSaveRes{Id: id}
	return
}

func (c *cPublish) TaskSubmit(ctx context.Context, req *publish.TaskSubmitReq) (res *publish.TaskSubmitRes, err error) {
	err = service.SysPublish().AdminTaskSubmit(ctx, &req.TaskSubmitInp)
	if err != nil {
		return
	}
	res = &publish.TaskSubmitRes{}
	return
}

func (c *cPublish) TaskCancel(ctx context.Context, req *publish.TaskCancelReq) (res *publish.TaskCancelRes, err error) {
	err = service.SysPublish().AdminTaskCancel(ctx, &req.TaskCancelInp)
	if err != nil {
		return
	}
	res = &publish.TaskCancelRes{}
	return
}
