package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishAdmin) ChannelClearQueue(ctx context.Context, req *publish.AdminChannelClearQueueReq) (res *publish.AdminChannelClearQueueRes, err error) {
	item, err := service.SysPublish().AdminChannelClearQueue(ctx, &req.ChannelClearQueueInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminChannelClearQueueRes{ChannelClearQueueModel: item}
	return
}
