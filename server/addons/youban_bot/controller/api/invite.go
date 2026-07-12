package api

import (
	"context"

	botapi "hotgo/addons/youban_bot/api/api/bot"
	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/addons/youban_bot/service"
)

func (c *cBotAuth) InviteInfo(ctx context.Context, req *botapi.InviteInfoReq) (res *botapi.InviteInfoRes, err error) {
	data, err := service.SysBot().MyInviteInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &botapi.InviteInfoRes{InviteInfoModel: data}, nil
}

func (c *cBotAuth) InviteList(ctx context.Context, req *botapi.InviteListReq) (res *botapi.InviteListRes, err error) {
	list, totalCount, err := service.SysBot().MyInviteList(ctx, &req.InviteListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.InviteModel{}
	}
	res = new(botapi.InviteListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cBotAuth) InviteGenerate(ctx context.Context, req *botapi.InviteGenerateReq) (res *botapi.InviteGenerateRes, err error) {
	data, err := service.SysBot().CreateInviteCode(ctx, &req.InviteCreateInp)
	if err != nil {
		return nil, err
	}
	return &botapi.InviteGenerateRes{InviteCreateModel: data}, nil
}
