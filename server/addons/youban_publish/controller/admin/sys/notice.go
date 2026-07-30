package sys

import (
	"context"

	"hotgo/addons/youban_publish/api/admin/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishServer) NoticeList(ctx context.Context, req *publish.NoticeListReq) (res *publish.NoticeListRes, err error) {
	list, totalCount, err := service.SysPublish().NoticeList(ctx, &req.NoticeListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.NoticeListModel{}
	}
	res = new(publish.NoticeListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) NoticeView(ctx context.Context, req *publish.NoticeViewReq) (res *publish.NoticeViewRes, err error) {
	data, err := service.SysPublish().NoticeView(ctx, &req.NoticeViewInp)
	if err != nil {
		return nil, err
	}
	return &publish.NoticeViewRes{NoticeViewModel: data}, nil
}

func (c *cPublishServer) NoticeEdit(ctx context.Context, req *publish.NoticeEditReq) (res *publish.NoticeEditRes, err error) {
	err = service.SysPublish().NoticeEdit(ctx, &req.NoticeEditInp)
	return
}

func (c *cPublishServer) NoticeDelete(ctx context.Context, req *publish.NoticeDeleteReq) (res *publish.NoticeDeleteRes, err error) {
	err = service.SysPublish().NoticeDelete(ctx, &req.NoticeDeleteInp)
	return
}

func (c *cPublishServer) NoticeMaxSort(ctx context.Context, req *publish.NoticeMaxSortReq) (res *publish.NoticeMaxSortRes, err error) {
	data, err := service.SysPublish().NoticeMaxSort(ctx, &req.NoticeMaxSortInp)
	if err != nil {
		return nil, err
	}
	return &publish.NoticeMaxSortRes{NoticeMaxSortModel: data}, nil
}

func (c *cPublishServer) NoticeStatus(ctx context.Context, req *publish.NoticeStatusReq) (res *publish.NoticeStatusRes, err error) {
	err = service.SysPublish().NoticeStatus(ctx, &req.NoticeStatusInp)
	return
}

func (c *cPublishServer) NoticeEditNotify(ctx context.Context, req *publish.NoticeEditNotifyReq) (res *publish.NoticeEditNotifyRes, err error) {
	req.Type = 1
	err = service.SysPublish().NoticeEdit(ctx, &req.NoticeEditInp)
	return
}

func (c *cPublishServer) NoticeEditNotice(ctx context.Context, req *publish.NoticeEditNoticeReq) (res *publish.NoticeEditNoticeRes, err error) {
	req.Type = 2
	err = service.SysPublish().NoticeEdit(ctx, &req.NoticeEditInp)
	return
}

func (c *cPublishServer) NoticeEditLetter(ctx context.Context, req *publish.NoticeEditLetterReq) (res *publish.NoticeEditLetterRes, err error) {
	req.Type = 3
	err = service.SysPublish().NoticeEdit(ctx, &req.NoticeEditInp)
	return
}

func (c *cPublishServer) PullMessages(ctx context.Context, req *publish.PullMessagesReq) (res *publish.PullMessagesRes, err error) {
	data, err := service.SysPublish().NoticePullMessages(ctx, &req.PullMessagesInp)
	if err != nil {
		return nil, err
	}
	return &publish.PullMessagesRes{PullMessagesModel: data}, nil
}

func (c *cPublishServer) UpRead(ctx context.Context, req *publish.UpReadReq) (res *publish.UpReadRes, err error) {
	err = service.SysPublish().NoticeUpRead(ctx, req.Id)
	return
}

func (c *cPublishServer) ReadAll(ctx context.Context, req *publish.ReadAllReq) (res *publish.ReadAllRes, err error) {
	err = service.SysPublish().NoticeReadAll(ctx, &req.NoticeReadAllInp)
	return
}

func (c *cPublishServer) MessageList(ctx context.Context, req *publish.MessageListReq) (res *publish.MessageListRes, err error) {
	list, totalCount, err := service.SysPublish().NoticeMessageList(ctx, &req.NoticeMessageListInp)
	if err != nil {
		return nil, err
	}
	res = new(publish.MessageListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}
