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
