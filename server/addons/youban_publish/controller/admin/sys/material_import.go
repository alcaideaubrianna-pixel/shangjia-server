package sys

import (
	"context"

	"hotgo/addons/youban_publish/api/admin/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

var MaterialImport = cMaterialImportAdmin{}

type cMaterialImportAdmin struct{}

func (c *cMaterialImportAdmin) TaskList(ctx context.Context, req *publish.MaterialImportTaskListReq) (res *publish.MaterialImportTaskListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminMaterialImportTaskList(ctx, &req.MaterialImportListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.MaterialImportTaskModel{}
	}
	res = new(publish.MaterialImportTaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cMaterialImportAdmin) TaskCreate(ctx context.Context, req *publish.MaterialImportTaskCreateReq) (res *publish.MaterialImportTaskCreateRes, err error) {
	id, err := service.SysPublish().AdminMaterialImportTaskCreate(ctx, &req.MaterialImportTaskSaveInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MaterialImportTaskCreateRes{Id: id}
	return
}

func (c *cMaterialImportAdmin) TaskView(ctx context.Context, req *publish.MaterialImportTaskViewReq) (res *publish.MaterialImportTaskViewRes, err error) {
	data, err := service.SysPublish().AdminMaterialImportTaskView(ctx, &req.MaterialImportTaskViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MaterialImportTaskViewRes{MaterialImportTaskModel: data}
	return
}

func (c *cMaterialImportAdmin) TaskStart(ctx context.Context, req *publish.MaterialImportTaskStartReq) (res *publish.MaterialImportTaskStartRes, err error) {
	if err = service.SysPublish().AdminMaterialImportTaskStart(ctx, &req.MaterialImportTaskActionInp); err != nil {
		return nil, err
	}
	res = &publish.MaterialImportTaskStartRes{}
	return
}

func (c *cMaterialImportAdmin) TaskCancel(ctx context.Context, req *publish.MaterialImportTaskCancelReq) (res *publish.MaterialImportTaskCancelRes, err error) {
	if err = service.SysPublish().AdminMaterialImportTaskCancel(ctx, &req.MaterialImportTaskActionInp); err != nil {
		return nil, err
	}
	res = &publish.MaterialImportTaskCancelRes{}
	return
}

func (c *cMaterialImportAdmin) TaskRetry(ctx context.Context, req *publish.MaterialImportTaskRetryReq) (res *publish.MaterialImportTaskRetryRes, err error) {
	if err = service.SysPublish().AdminMaterialImportTaskRetry(ctx, &req.MaterialImportTaskActionInp); err != nil {
		return nil, err
	}
	res = &publish.MaterialImportTaskRetryRes{}
	return
}
