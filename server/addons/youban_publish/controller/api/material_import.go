package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishAdmin) MaterialImportTaskList(ctx context.Context, req *publish.AdminMaterialImportTaskListReq) (res *publish.AdminMaterialImportTaskListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminMaterialImportTaskList(ctx, &req.MaterialImportListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.MaterialImportTaskModel{}
	}
	res = new(publish.AdminMaterialImportTaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) MaterialImportTaskCreate(ctx context.Context, req *publish.AdminMaterialImportTaskCreateReq) (res *publish.AdminMaterialImportTaskCreateRes, err error) {
	id, err := service.SysPublish().AdminMaterialImportTaskCreate(ctx, &req.MaterialImportTaskSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminMaterialImportTaskCreateRes{Id: id}
	return
}

func (c *cPublishAdmin) MaterialImportTaskView(ctx context.Context, req *publish.AdminMaterialImportTaskViewReq) (res *publish.AdminMaterialImportTaskViewRes, err error) {
	data, err := service.SysPublish().AdminMaterialImportTaskView(ctx, &req.MaterialImportTaskViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminMaterialImportTaskViewRes{MaterialImportTaskModel: data}
	return
}

func (c *cPublishAdmin) MaterialImportTaskStart(ctx context.Context, req *publish.AdminMaterialImportTaskStartReq) (res *publish.AdminMaterialImportTaskStartRes, err error) {
	if err = service.SysPublish().AdminMaterialImportTaskStart(ctx, &req.MaterialImportTaskActionInp); err != nil {
		return nil, err
	}
	res = &publish.AdminMaterialImportTaskStartRes{}
	return
}

func (c *cPublishAdmin) MaterialImportTaskCancel(ctx context.Context, req *publish.AdminMaterialImportTaskCancelReq) (res *publish.AdminMaterialImportTaskCancelRes, err error) {
	if err = service.SysPublish().AdminMaterialImportTaskCancel(ctx, &req.MaterialImportTaskActionInp); err != nil {
		return nil, err
	}
	res = &publish.AdminMaterialImportTaskCancelRes{}
	return
}

func (c *cPublishAdmin) MaterialImportTaskRetry(ctx context.Context, req *publish.AdminMaterialImportTaskRetryReq) (res *publish.AdminMaterialImportTaskRetryRes, err error) {
	if err = service.SysPublish().AdminMaterialImportTaskRetry(ctx, &req.MaterialImportTaskActionInp); err != nil {
		return nil, err
	}
	res = &publish.AdminMaterialImportTaskRetryRes{}
	return
}
