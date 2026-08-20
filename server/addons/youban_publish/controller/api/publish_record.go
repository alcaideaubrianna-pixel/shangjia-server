package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

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

func (c *cPublishAdmin) InclusionRecordList(ctx context.Context, req *publish.AdminInclusionRecordListReq) (*publish.AdminInclusionRecordListRes, error) {
	list, total, err := service.SysPublish().AdminInclusionRecordList(ctx, &req.InclusionRecordListInp)
	if err != nil {
		return nil, err
	}
	res := &publish.AdminInclusionRecordListRes{List: list}
	res.PageRes.Pack(req, total)
	return res, nil
}

func (c *cPublishAdmin) TgObserveQueueList(ctx context.Context, req *publish.AdminTgObserveQueueListReq) (res *publish.AdminTgObserveQueueListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTgObserveQueueList(ctx, &req.TgObserveQueueListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgObserveQueueStatModel{}
	}
	res = new(publish.AdminTgObserveQueueListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TgObserveChannelList(ctx context.Context, req *publish.AdminTgObserveChannelListReq) (res *publish.AdminTgObserveChannelListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTgObserveChannelList(ctx, &req.TgObserveChannelListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgObserveChannelStatModel{}
	}
	res = new(publish.AdminTgObserveChannelListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishAdmin) TgObserveBotList(ctx context.Context, req *publish.AdminTgObserveBotListReq) (res *publish.AdminTgObserveBotListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminTgObserveBotList(ctx, &req.TgObserveBotListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgObserveBotStatModel{}
	}
	res = new(publish.AdminTgObserveBotListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
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

func (c *cPublish) MyInclusionRecordList(ctx context.Context, req *publish.MyInclusionRecordListReq) (*publish.MyInclusionRecordListRes, error) {
	list, total, err := service.SysPublish().MyInclusionRecordList(ctx, &req.InclusionRecordListInp)
	if err != nil {
		return nil, err
	}
	res := &publish.MyInclusionRecordListRes{List: list}
	res.PageRes.Pack(req, total)
	return res, nil
}

func (c *cPublish) MyDevPublishChainTest(ctx context.Context, req *publish.MyDevPublishChainTestReq) (res *publish.MyDevPublishChainTestRes, err error) {
	data, err := service.SysPublish().MyDevPublishChainTest(ctx, &req.DevPublishChainTestInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MyDevPublishChainTestRes{DevPublishChainTestModel: data}
	return
}
