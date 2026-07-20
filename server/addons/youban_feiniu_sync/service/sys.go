package service

import (
	"context"
	"hotgo/addons/youban_feiniu_sync/model/input/sysin"
)

type ISysSync interface {
	TenantOptions(ctx context.Context, in *sysin.OptionListInp) (list []*sysin.TenantOptionModel, err error)
	AdminAccountOptions(ctx context.Context, in *sysin.OptionListInp) (list []*sysin.AccountOptionModel, err error)
	AccountOptions(ctx context.Context, in *sysin.OptionListInp) (list []*sysin.AccountOptionModel, err error)
	ConfigList(ctx context.Context, in *sysin.ConfigListInp) (list []*sysin.ConfigModel, totalCount int, err error)
	ConfigView(ctx context.Context, in *sysin.ConfigViewInp) (res *sysin.ConfigModel, err error)
	ConfigSave(ctx context.Context, in *sysin.ConfigSaveInp) (res *sysin.ConfigModel, err error)
	ConfigDelete(ctx context.Context, in *sysin.ConfigDeleteInp) error
	ConfigAutoSync(ctx context.Context, in *sysin.ConfigAutoSyncInp) error
	ConfigTest(ctx context.Context, in *sysin.ConfigTestInp) (res *sysin.ConfigTestModel, err error)
	Dashboard(ctx context.Context, in *sysin.DashboardInp) (res *sysin.DashboardModel, err error)
	DashboardSummary(ctx context.Context, in *sysin.DashboardInp) (res *sysin.DashboardSummaryModel, err error)
	DashboardTrend(ctx context.Context, in *sysin.DashboardInp) (list []*sysin.DashboardTrendModel, err error)
	DashboardChannelRank(ctx context.Context, in *sysin.DashboardInp, limit int) (list []*sysin.DashboardChannelRankModel, err error)
	DashboardRecentRuns(ctx context.Context, in *sysin.DashboardInp, limit int) (list []*sysin.RunModel, err error)
	ChannelMapList(ctx context.Context, in *sysin.ChannelMapListInp) (list []*sysin.ChannelMapModel, totalCount int, err error)
	ChannelClear(ctx context.Context, in *sysin.ChannelClearInp) (res *sysin.ChannelClearModel, err error)
	ChannelCopy(ctx context.Context, in *sysin.ChannelCopyInp) (res *sysin.ChannelCopyModel, err error)
	ChannelDisable(ctx context.Context, in *sysin.ChannelDisableInp) (res *sysin.ChannelDisableModel, err error)
	RunList(ctx context.Context, in *sysin.RunListInp) (list []*sysin.RunModel, totalCount int, err error)
	RunView(ctx context.Context, in *sysin.RunViewInp) (res *sysin.RunModel, err error)
	RunItemList(ctx context.Context, in *sysin.RunItemListInp) (list []*sysin.RunItemModel, totalCount int, err error)
	StartRun(ctx context.Context, in *sysin.RunStartInp) (res *sysin.RunStartModel, err error)
	CronRun(ctx context.Context) error
	CronRunConfig(ctx context.Context, configId int64) error
}

var localSysSync ISysSync

func SysSync() ISysSync {
	if localSysSync == nil {
		panic("implement not found for interface ISysSync, forgot register?")
	}
	return localSysSync
}
func RegisterSysSync(i ISysSync) { localSysSync = i }
