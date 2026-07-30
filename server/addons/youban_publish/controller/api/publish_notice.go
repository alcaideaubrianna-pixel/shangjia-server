package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublish) PullMessages(ctx context.Context, req *publish.PullMessagesReq) (res *publish.PullMessagesRes, err error) {
	data, err := service.SysPublish().NoticePullMessages(ctx, &req.PullMessagesInp)
	if err != nil {
		return nil, err
	}
	return &publish.PullMessagesRes{PullMessagesModel: data}, nil
}

func (c *cPublish) UpRead(ctx context.Context, req *publish.UpReadReq) (res *publish.UpReadRes, err error) {
	err = service.SysPublish().NoticeUpRead(ctx, req.Id)
	return
}

func (c *cPublish) ReadAll(ctx context.Context, req *publish.ReadAllReq) (res *publish.ReadAllRes, err error) {
	err = service.SysPublish().NoticeReadAll(ctx, &req.NoticeReadAllInp)
	return
}

func (c *cPublish) MessageList(ctx context.Context, req *publish.MessageListReq) (res *publish.MessageListRes, err error) {
	list, totalCount, err := service.SysPublish().NoticeMessageList(ctx, &req.NoticeMessageListInp)
	if err != nil {
		return nil, err
	}
	res = new(publish.MessageListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}
