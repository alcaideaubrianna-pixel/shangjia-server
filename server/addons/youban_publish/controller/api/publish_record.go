package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishAdmin) SubmitTask(ctx context.Context, req *publish.AdminSubmitTaskReq) (res *publish.AdminSubmitTaskRes, err error) {
	if err = service.SysPublish().AdminTaskSubmit(ctx, &req.TaskSubmitInp); err != nil {
		return nil, err
	}
	res = &publish.AdminSubmitTaskRes{}
	return
}

func (c *cPublishAdmin) PublishRecordList(ctx context.Context, req *publish.AdminPublishRecordListReq) (res *publish.AdminPublishRecordListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminPublishRecordList(ctx, &req.PublishRecordListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.PublishRecordModel{}
	}
	res = new(publish.AdminPublishRecordListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) PublishRecordClear(ctx context.Context, req *publish.AdminPublishRecordClearReq) (res *publish.AdminPublishRecordClearRes, err error) {
	if err = service.SysPublish().AdminPublishRecordClear(ctx, &req.PublishRecordClearInp); err != nil {
		return nil, err
	}
	res = &publish.AdminPublishRecordClearRes{}
	return
}

func (c *cPublishAdmin) DevPublishChainTest(ctx context.Context, req *publish.AdminDevPublishChainTestReq) (res *publish.AdminDevPublishChainTestRes, err error) {
	data, err := service.SysPublish().AdminDevPublishChainTest(ctx, &req.DevPublishChainTestInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminDevPublishChainTestRes{DevPublishChainTestModel: data}
	return
}

func (c *cPublish) MyPublishRecordList(ctx context.Context, req *publish.MyPublishRecordListReq) (res *publish.MyPublishRecordListRes, err error) {
	list, totalCount, err := service.SysPublish().MyPublishRecordList(ctx, &req.PublishRecordListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.PublishRecordModel{}
	}
	res = new(publish.MyPublishRecordListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) MyPublishRecordClear(ctx context.Context, req *publish.MyPublishRecordClearReq) (res *publish.MyPublishRecordClearRes, err error) {
	if err = service.SysPublish().MyPublishRecordClear(ctx, &req.PublishRecordClearInp); err != nil {
		return nil, err
	}
	res = &publish.MyPublishRecordClearRes{}
	return
}

func (c *cPublish) MyDevPublishChainTest(ctx context.Context, req *publish.MyDevPublishChainTestReq) (res *publish.MyDevPublishChainTestRes, err error) {
	data, err := service.SysPublish().MyDevPublishChainTest(ctx, &req.DevPublishChainTestInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyDevPublishChainTestRes{DevPublishChainTestModel: data}
	return
}
