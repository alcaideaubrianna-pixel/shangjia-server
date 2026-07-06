package sys

import (
	"context"

	"hotgo/addons/youban_publish/api/admin/publish"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishServer) Dashboard(ctx context.Context, req *publish.DashboardReq) (res *publish.DashboardRes, err error) {
	data, err := service.SysPublish().ServerDashboard(ctx, &req.TrendInp)
	if err != nil {
		return nil, err
	}
	return &publish.DashboardRes{ServerDashboardModel: data}, nil
}
