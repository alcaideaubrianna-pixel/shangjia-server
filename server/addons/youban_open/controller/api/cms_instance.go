package api

import (
	"context"
	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/addons/youban_open/api/api/open"
	"hotgo/addons/youban_open/service"
)

var CmsInstance = cCmsInstance{}

type cCmsInstance struct{}

func (c *cCmsInstance) Register(ctx context.Context, req *open.InstanceRegisterReq) (*open.InstanceRegisterRes, error) {
	data, err := service.OpenAccess().RegisterInstance(ctx, &req.CmsInstanceRegisterInp, ghttp.RequestFromCtx(ctx).GetClientIp())
	if err != nil {
		return nil, err
	}
	return &open.InstanceRegisterRes{CmsInstanceRegisterModel: data}, nil
}

func (c *cCmsInstance) Heartbeat(ctx context.Context, req *open.InstanceHeartbeatReq) (*open.InstanceHeartbeatRes, error) {
	data, err := service.OpenAccess().HeartbeatInstance(ctx, &req.CmsInstanceHeartbeatInp, ghttp.RequestFromCtx(ctx).GetClientIp())
	if err != nil {
		return nil, err
	}
	return &open.InstanceHeartbeatRes{CmsInstanceRegisterModel: data}, nil
}
