package service

import (
	"context"
	"hotgo/addons/youban_invite/model/input/sysin"
)

type ISysInvite interface {
	Stats(ctx context.Context) (res *sysin.InviteStatsModel, err error)
	Ledger(ctx context.Context, in *sysin.InviteLedgerInp) (list []*sysin.InviteRecordModel, totalCount int, err error)
	AdminConfig(ctx context.Context) (res *sysin.InviteConfigModel, err error)
	AdminSaveConfig(ctx context.Context, in *sysin.InviteConfigSaveInp) (err error)
	AdminList(ctx context.Context, in *sysin.InviteRecordListInp) (list []*sysin.InviteRecordModel, totalCount int, err error)
	AdminSaveRecord(ctx context.Context, in *sysin.InviteRecordSaveInp) (err error)
	AdminDelete(ctx context.Context, in *sysin.InviteRecordDeleteInp) (err error)
}

var localSysInvite ISysInvite

func SysInvite() ISysInvite {
	if localSysInvite == nil {
		panic("implement not found for interface ISysInvite, forgot register?")
	}
	return localSysInvite
}

func RegisterSysInvite(i ISysInvite) {
	localSysInvite = i
}
