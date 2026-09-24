package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishAdmin) ListenerPlanList(ctx context.Context, req *publish.AdminListenerPlanListReq) (res *publish.AdminListenerPlanListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminListenerPlanList(ctx, &req.ListenerPlanListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ListenerPlanModel{}
	}
	res = &publish.AdminListenerPlanListRes{}
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) AccountListenerPlanList(ctx context.Context, req *publish.AccountListenerPlanListReq) (res *publish.AccountListenerPlanListRes, err error) {
	list, totalCount, err := service.SysPublish().AccountListenerPlanList(ctx, &req.ListenerPlanListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.ListenerPlanModel{}
	}
	res = &publish.AccountListenerPlanListRes{List: list}
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) AccountListenerPlanSave(ctx context.Context, req *publish.AccountListenerPlanSaveReq) (*publish.AccountListenerPlanSaveRes, error) {
	data, err := service.SysPublish().AdminListenerPlanSave(ctx, &req.ListenerPlanSaveInp)
	if err != nil {
		return nil, err
	}
	return &publish.AccountListenerPlanSaveRes{Id: data.Id}, nil
}

func (c *cPublish) AccountListenerPlanDelete(ctx context.Context, req *publish.AccountListenerPlanDeleteReq) (*publish.AccountListenerPlanDeleteRes, error) {
	if err := service.SysPublish().AdminListenerPlanDelete(ctx, &req.ListenerPlanDeleteInp); err != nil {
		return nil, err
	}
	return &publish.AccountListenerPlanDeleteRes{}, nil
}

func (c *cPublish) AccountListenerPlanStatus(ctx context.Context, req *publish.AccountListenerPlanStatusReq) (*publish.AccountListenerPlanStatusRes, error) {
	if err := service.SysPublish().AdminListenerPlanStatus(ctx, &req.ListenerPlanStatusInp); err != nil {
		return nil, err
	}
	return &publish.AccountListenerPlanStatusRes{}, nil
}

func (c *cPublish) AccountListenerPlanUnbind(ctx context.Context, req *publish.AccountListenerPlanUnbindReq) (*publish.AccountListenerPlanUnbindRes, error) {
	if err := service.SysPublish().AdminListenerPlanUnbind(ctx, &req.ListenerPlanUnbindInp); err != nil {
		return nil, err
	}
	return &publish.AccountListenerPlanUnbindRes{}, nil
}

func (c *cPublishAdmin) ListenerPlanSave(ctx context.Context, req *publish.AdminListenerPlanSaveReq) (res *publish.AdminListenerPlanSaveRes, err error) {
	data, err := service.SysPublish().AdminListenerPlanSave(ctx, &req.ListenerPlanSaveInp)
	if err != nil {
		return nil, err
	}
	if data == nil {
		data = &sysin.ListenerPlanSaveModel{}
	}
	res = &publish.AdminListenerPlanSaveRes{Id: data.Id}
	return
}

func (c *cPublishAdmin) ListenerPlanDelete(ctx context.Context, req *publish.AdminListenerPlanDeleteReq) (res *publish.AdminListenerPlanDeleteRes, err error) {
	if err = service.SysPublish().AdminListenerPlanDelete(ctx, &req.ListenerPlanDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminListenerPlanDeleteRes{}
	return
}

func (c *cPublishAdmin) ListenerPlanStatus(ctx context.Context, req *publish.AdminListenerPlanStatusReq) (res *publish.AdminListenerPlanStatusRes, err error) {
	if err = service.SysPublish().AdminListenerPlanStatus(ctx, &req.ListenerPlanStatusInp); err != nil {
		return nil, err
	}
	res = &publish.AdminListenerPlanStatusRes{}
	return
}

func (c *cPublishAdmin) ListenerPlanUnbind(ctx context.Context, req *publish.AdminListenerPlanUnbindReq) (res *publish.AdminListenerPlanUnbindRes, err error) {
	if err = service.SysPublish().AdminListenerPlanUnbind(ctx, &req.ListenerPlanUnbindInp); err != nil {
		return nil, err
	}
	res = &publish.AdminListenerPlanUnbindRes{}
	return
}
