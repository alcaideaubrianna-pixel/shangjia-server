package sys

import (
	"context"

	"hotgo/addons/youban_publish/api/admin/config"
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
