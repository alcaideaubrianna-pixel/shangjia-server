package sys

import (
	"context"

	"hotgo/addons/youban_publish/api/admin/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishServer) ChannelMemberSyncStart(ctx context.Context, req *publish.ChannelMemberSyncStartReq) (res *publish.ChannelMemberSyncStartRes, err error) {
	data, err := service.SysPublish().AdminChannelMemberSyncStart(ctx, &req.TgChannelMemberSyncStartInp)
	if err != nil {
		return nil, err
	}
	return &publish.ChannelMemberSyncStartRes{TgChannelMemberSyncModel: data}, nil
}

func (c *cPublishServer) ChannelMemberSyncView(ctx context.Context, req *publish.ChannelMemberSyncViewReq) (res *publish.ChannelMemberSyncViewRes, err error) {
	data, err := service.SysPublish().AdminChannelMemberSyncView(ctx, &req.TgChannelMemberSyncViewInp)
	if err != nil {
		return nil, err
	}
	return &publish.ChannelMemberSyncViewRes{TgChannelMemberSyncModel: data}, nil
}

func (c *cPublishServer) ChannelMemberSyncCancel(ctx context.Context, req *publish.ChannelMemberSyncCancelReq) (res *publish.ChannelMemberSyncCancelRes, err error) {
	if err = service.SysPublish().AdminChannelMemberSyncCancel(ctx, &req.TgChannelMemberSyncCancelInp); err != nil {
		return nil, err
	}
	return &publish.ChannelMemberSyncCancelRes{}, nil
}

func (c *cPublishServer) ChannelMemberList(ctx context.Context, req *publish.ChannelMemberListReq) (res *publish.ChannelMemberListRes, err error) {
	list, totalCount, err := service.SysPublish().AdminChannelMemberList(ctx, &req.TgChannelMemberListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TgChannelMemberModel{}
	}
	res = new(publish.ChannelMemberListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublishServer) ChannelMemberExport(ctx context.Context, req *publish.ChannelMemberExportReq) (res *publish.ChannelMemberExportRes, err error) {
	err = service.SysPublish().AdminChannelMemberExport(ctx, &req.TgChannelMemberListInp)
	return &publish.ChannelMemberExportRes{}, err
}
