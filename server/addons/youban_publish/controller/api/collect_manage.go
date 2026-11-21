package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublish) CollectManageChannelList(ctx context.Context, req *publish.CollectManageChannelListReq) (*publish.CollectManageChannelListRes, error) {
	list, total, err := service.SysPublish().CollectManageChannels(ctx, &req.CollectManageChannelListInp)
	if err != nil {
		return nil, err
	}
	res := &publish.CollectManageChannelListRes{List: list}
	res.PageRes.Pack(req, total)
	return res, nil
}

func (c *cPublish) CollectManageChannelAction(ctx context.Context, req *publish.CollectManageChannelActionReq) (*publish.CollectManageChannelActionRes, error) {
	if err := service.SysPublish().CollectManageChannelAction(ctx, &req.CollectManageChannelActionInp); err != nil {
		return nil, err
	}
	return &publish.CollectManageChannelActionRes{}, nil
}
