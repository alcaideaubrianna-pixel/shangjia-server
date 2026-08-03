package sys

import (
	"context"

	"hotgo/addons/youban_publish/api/admin/config"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

var Config = cConfig{}

type cConfig struct{}

func (c *cConfig) Get(ctx context.Context, req *config.GetReq) (res *config.GetRes, err error) {
	data, err := service.SysConfig().GetConfigByGroup(ctx, &req.GetConfigInp)
	if err != nil {
		return nil, err
	}
	res = &config.GetRes{GetConfigModel: data}
	return
}

func (c *cConfig) Update(ctx context.Context, req *config.UpdateReq) (res *config.UpdateRes, err error) {
	if err = service.SysConfig().UpdateConfigByGroup(ctx, &req.UpdateConfigInp); err != nil {
		return nil, err
	}
	res = &config.UpdateRes{}
	return
}

func (c *cConfig) CloudUsageDashboard(ctx context.Context, req *config.CloudUsageDashboardReq) (res *config.CloudUsageDashboardRes, err error) {
	data, err := service.SysConfig().CloudResourceUsageDashboard(ctx, &req.CloudResourceUsageDashboardInp)
	if err != nil {
		return nil, err
	}
	return &config.CloudUsageDashboardRes{CloudResourceUsageDashboardModel: data}, nil
}

func (c *cConfig) CloudUsageList(ctx context.Context, req *config.CloudUsageListReq) (res *config.CloudUsageListRes, err error) {
	list, totalCount, err := service.SysConfig().CloudResourceUsageList(ctx, &req.CloudResourceUsageListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.CloudResourceUsageModel{}
	}
	res = new(config.CloudUsageListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}
