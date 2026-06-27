package api

import (
	"context"

	"hotgo/addons/youban_invite/api/api/invite"
	"hotgo/addons/youban_invite/model/input/sysin"
	"hotgo/addons/youban_invite/service"
)

var Invite = cInvite{}

type cInvite struct{}

func (c *cInvite) Stats(ctx context.Context, req *invite.StatsReq) (res *invite.StatsRes, err error) {
	data, err := service.SysInvite().Stats(ctx)
	if err != nil {
		return
	}
	res = &invite.StatsRes{InviteStatsModel: data}
	return
}

func (c *cInvite) Ledger(ctx context.Context, req *invite.LedgerReq) (res *invite.LedgerRes, err error) {
	list, totalCount, err := service.SysInvite().Ledger(ctx, &req.InviteLedgerInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.InviteRecordModel{}
	}
	res = new(invite.LedgerRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}
