package sys

import (
	"context"
	syncapi "hotgo/addons/youban_feiniu_sync/api/admin/sync"
	"hotgo/addons/youban_feiniu_sync/service"
)

var Sync = cSync{}

type cSync struct{}

func (c *cSync) TenantOptions(ctx context.Context, req *syncapi.TenantOptionsReq) (res *syncapi.TenantOptionsRes, err error) {
	list, err := service.SysSync().TenantOptions(ctx, &req.OptionListInp)
	if err != nil {
		return
	}
	res = &syncapi.TenantOptionsRes{List: list}
	return
}
func (c *cSync) AdminAccountOptions(ctx context.Context, req *syncapi.AdminAccountOptionsReq) (res *syncapi.AdminAccountOptionsRes, err error) {
	list, err := service.SysSync().AdminAccountOptions(ctx, &req.OptionListInp)
	if err != nil {
		return
	}
	res = &syncapi.AdminAccountOptionsRes{List: list}
	return
}
func (c *cSync) ConfigList(ctx context.Context, req *syncapi.ConfigListReq) (res *syncapi.ConfigListRes, err error) {
	list, total, err := service.SysSync().ConfigList(ctx, &req.ConfigListInp)
	if err != nil {
		return
	}
	res = new(syncapi.ConfigListRes)
	res.List = list
	res.PageRes.Pack(req, total)
	return
}
func (c *cSync) ConfigView(ctx context.Context, req *syncapi.ConfigViewReq) (res *syncapi.ConfigViewRes, err error) {
	data, err := service.SysSync().ConfigView(ctx, &req.ConfigViewInp)
	if err != nil {
		return
	}
	res = &syncapi.ConfigViewRes{ConfigModel: data}
	return
}
func (c *cSync) ConfigSave(ctx context.Context, req *syncapi.ConfigSaveReq) (res *syncapi.ConfigSaveRes, err error) {
	data, err := service.SysSync().ConfigSave(ctx, &req.ConfigSaveInp)
	if err != nil {
		return
	}
	res = &syncapi.ConfigSaveRes{ConfigModel: data}
	return
}
func (c *cSync) ConfigDelete(ctx context.Context, req *syncapi.ConfigDeleteReq) (res *syncapi.ConfigDeleteRes, err error) {
	err = service.SysSync().ConfigDelete(ctx, &req.ConfigDeleteInp)
	return
}
func (c *cSync) ConfigAutoSync(ctx context.Context, req *syncapi.ConfigAutoSyncReq) (res *syncapi.ConfigAutoSyncRes, err error) {
	err = service.SysSync().ConfigAutoSync(ctx, &req.ConfigAutoSyncInp)
	return
}
func (c *cSync) ConfigTest(ctx context.Context, req *syncapi.ConfigTestReq) (res *syncapi.ConfigTestRes, err error) {
	data, err := service.SysSync().ConfigTest(ctx, &req.ConfigTestInp)
	if err != nil {
		return
	}
	res = &syncapi.ConfigTestRes{ConfigTestModel: data}
	return
}
func (c *cSync) Dashboard(ctx context.Context, req *syncapi.DashboardReq) (res *syncapi.DashboardRes, err error) {
	data, err := service.SysSync().Dashboard(ctx, &req.DashboardInp)
	if err != nil {
		return
	}
	res = &syncapi.DashboardRes{DashboardModel: data}
	return
}
func (c *cSync) DashboardSummary(ctx context.Context, req *syncapi.DashboardSummaryReq) (res *syncapi.DashboardSummaryRes, err error) {
	data, err := service.SysSync().DashboardSummary(ctx, &req.DashboardInp)
	if err != nil {
		return
	}
	res = &syncapi.DashboardSummaryRes{DashboardSummaryModel: data}
	return
}
func (c *cSync) DashboardTrend(ctx context.Context, req *syncapi.DashboardTrendReq) (res *syncapi.DashboardTrendRes, err error) {
	list, err := service.SysSync().DashboardTrend(ctx, &req.DashboardInp)
	if err != nil {
		return
	}
	res = &syncapi.DashboardTrendRes{List: list}
	return
}
func (c *cSync) DashboardChannelRank(ctx context.Context, req *syncapi.DashboardChannelRankReq) (res *syncapi.DashboardChannelRankRes, err error) {
	list, err := service.SysSync().DashboardChannelRank(ctx, &req.DashboardInp, req.Limit)
	if err != nil {
		return
	}
	res = &syncapi.DashboardChannelRankRes{List: list}
	return
}
func (c *cSync) DashboardRecentRuns(ctx context.Context, req *syncapi.DashboardRecentRunsReq) (res *syncapi.DashboardRecentRunsRes, err error) {
	list, err := service.SysSync().DashboardRecentRuns(ctx, &req.DashboardInp, req.Limit)
	if err != nil {
		return
	}
	res = &syncapi.DashboardRecentRunsRes{List: list}
	return
}
func (c *cSync) ChannelMapList(ctx context.Context, req *syncapi.ChannelMapListReq) (res *syncapi.ChannelMapListRes, err error) {
	list, total, err := service.SysSync().ChannelMapList(ctx, &req.ChannelMapListInp)
	if err != nil {
		return
	}
	res = new(syncapi.ChannelMapListRes)
	res.List = list
	res.PageRes.Pack(req, total)
	return
}
func (c *cSync) ChannelClear(ctx context.Context, req *syncapi.ChannelClearReq) (res *syncapi.ChannelClearRes, err error) {
	data, err := service.SysSync().ChannelClear(ctx, &req.ChannelClearInp)
	if err != nil {
		return
	}
	res = &syncapi.ChannelClearRes{ChannelClearModel: data}
	return
}
func (c *cSync) RunList(ctx context.Context, req *syncapi.RunListReq) (res *syncapi.RunListRes, err error) {
	list, total, err := service.SysSync().RunList(ctx, &req.RunListInp)
	if err != nil {
		return
	}
	res = new(syncapi.RunListRes)
	res.List = list
	res.PageRes.Pack(req, total)
	return
}
func (c *cSync) RunView(ctx context.Context, req *syncapi.RunViewReq) (res *syncapi.RunViewRes, err error) {
	data, err := service.SysSync().RunView(ctx, &req.RunViewInp)
	if err != nil {
		return
	}
	res = &syncapi.RunViewRes{RunModel: data}
	return
}
func (c *cSync) RunItemList(ctx context.Context, req *syncapi.RunItemListReq) (res *syncapi.RunItemListRes, err error) {
	list, total, err := service.SysSync().RunItemList(ctx, &req.RunItemListInp)
	if err != nil {
		return
	}
	res = new(syncapi.RunItemListRes)
	res.List = list
	res.PageRes.Pack(req, total)
	return
}
func (c *cSync) RunStart(ctx context.Context, req *syncapi.RunStartReq) (res *syncapi.RunStartRes, err error) {
	data, err := service.SysSync().StartRun(ctx, &req.RunStartInp)
	if err != nil {
		return
	}
	res = &syncapi.RunStartRes{RunStartModel: data}
	return
}
