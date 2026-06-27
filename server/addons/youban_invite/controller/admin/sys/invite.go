package sys

import (
	"context"

	"hotgo/addons/youban_invite/api/admin/invite"
	"hotgo/addons/youban_invite/model/input/sysin"
	"hotgo/addons/youban_invite/service"
)

var Invite = cInvite{}

type cInvite struct{}

func (c *cInvite) Config(ctx context.Context, req *invite.ConfigReq) (res *invite.ConfigRes, err error) {
	data, err := service.SysInvite().AdminConfig(ctx)
	if err != nil {
		return
	}
	res = &invite.ConfigRes{InviteConfigModel: data}
	return
}

func (c *cInvite) SaveConfig(ctx context.Context, req *invite.SaveConfigReq) (res *invite.SaveConfigRes, err error) {
	err = service.SysInvite().AdminSaveConfig(ctx, &req.InviteConfigSaveInp)
	if err != nil {
		return
	}
	res = &invite.SaveConfigRes{}
	return
}

func (c *cInvite) List(ctx context.Context, req *invite.ListReq) (res *invite.ListRes, err error) {
	list, totalCount, err := service.SysInvite().AdminList(ctx, &req.InviteRecordListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.InviteRecordModel{}
	}
	res = new(invite.ListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cInvite) SaveRecord(ctx context.Context, req *invite.SaveRecordReq) (res *invite.SaveRecordRes, err error) {
	err = service.SysInvite().AdminSaveRecord(ctx, &req.InviteRecordSaveInp)
	if err != nil {
		return
	}
	res = &invite.SaveRecordRes{}
	return
}

func (c *cInvite) Delete(ctx context.Context, req *invite.DeleteReq) (res *invite.DeleteRes, err error) {
	err = service.SysInvite().AdminDelete(ctx, &req.InviteRecordDeleteInp)
	if err != nil {
		return
	}
	res = &invite.DeleteRes{}
	return
}
