package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

var Publish = cPublish{}

type cPublish struct{}

func (c *cPublish) CurrentAccount(ctx context.Context, req *publish.CurrentAccountReq) (res *publish.CurrentAccountRes, err error) {
	data, err := service.SysPublish().CurrentAccount(ctx)
	if err != nil {
		return
	}
	res = &publish.CurrentAccountRes{CurrentAccountModel: data}
	return
}

func (c *cPublish) MyTaskList(ctx context.Context, req *publish.MyTaskListReq) (res *publish.MyTaskListRes, err error) {
	list, totalCount, err := service.SysPublish().MyTaskList(ctx, &req.TaskListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.TaskModel{}
	}
	res = new(publish.MyTaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) SaveTask(ctx context.Context, req *publish.SaveTaskReq) (res *publish.SaveTaskRes, err error) {
	id, err := service.SysPublish().MyTaskSave(ctx, &req.TaskSaveInp)
	if err != nil {
		return
	}
	res = &publish.SaveTaskRes{Id: id}
	return
}

func (c *cPublish) SubmitTask(ctx context.Context, req *publish.SubmitTaskReq) (res *publish.SubmitTaskRes, err error) {
	err = service.SysPublish().MyTaskSubmit(ctx, &req.TaskSubmitInp)
	if err != nil {
		return
	}
	res = &publish.SubmitTaskRes{}
	return
}

func (c *cPublish) CancelTask(ctx context.Context, req *publish.CancelTaskReq) (res *publish.CancelTaskRes, err error) {
	err = service.SysPublish().MyTaskCancel(ctx, &req.TaskCancelInp)
	if err != nil {
		return
	}
	res = &publish.CancelTaskRes{}
	return
}
