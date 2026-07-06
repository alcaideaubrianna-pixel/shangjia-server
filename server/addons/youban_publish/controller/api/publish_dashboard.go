package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishAdmin) DashboardOverview(ctx context.Context, req *publish.AdminDashboardOverviewReq) (res *publish.AdminDashboardOverviewRes, err error) {
	data, err := service.SysPublish().AdminDashboardOverview(ctx)
	if err != nil {
		return nil, err
	}
	return &publish.AdminDashboardOverviewRes{DashboardOverviewModel: data}, nil
}

func (c *cPublishAdmin) DashboardTrend(ctx context.Context, req *publish.AdminDashboardTrendReq) (res *publish.AdminDashboardTrendRes, err error) {
	data, err := service.SysPublish().AdminDashboardTrend(ctx, &req.TrendInp)
	if err != nil {
		return nil, err
	}
	return &publish.AdminDashboardTrendRes{DashboardTrendModel: data}, nil
}

func (c *cPublishAdmin) DashboardTodo(ctx context.Context, req *publish.AdminDashboardTodoReq) (res *publish.AdminDashboardTodoRes, err error) {
	data, err := service.SysPublish().AdminDashboardTodo(ctx)
	if err != nil {
		return nil, err
	}
	return &publish.AdminDashboardTodoRes{DashboardTodoModel: data}, nil
}

func (c *cPublishAdmin) DashboardRank(ctx context.Context, req *publish.AdminDashboardRankReq) (res *publish.AdminDashboardRankRes, err error) {
	data, err := service.SysPublish().AdminDashboardRank(ctx)
	if err != nil {
		return nil, err
	}
	return &publish.AdminDashboardRankRes{DashboardRankModel: data}, nil
}

func (c *cPublish) DashboardOverview(ctx context.Context, req *publish.MyDashboardOverviewReq) (res *publish.MyDashboardOverviewRes, err error) {
	data, err := service.SysPublish().MyDashboardOverview(ctx)
	if err != nil {
		return nil, err
	}
	return &publish.MyDashboardOverviewRes{DashboardOverviewModel: data}, nil
}

func (c *cPublish) DashboardTrend(ctx context.Context, req *publish.MyDashboardTrendReq) (res *publish.MyDashboardTrendRes, err error) {
	data, err := service.SysPublish().MyDashboardTrend(ctx, &req.TrendInp)
	if err != nil {
		return nil, err
	}
	return &publish.MyDashboardTrendRes{DashboardTrendModel: data}, nil
}

func (c *cPublish) DashboardTodo(ctx context.Context, req *publish.MyDashboardTodoReq) (res *publish.MyDashboardTodoRes, err error) {
	data, err := service.SysPublish().MyDashboardTodo(ctx)
	if err != nil {
		return nil, err
	}
	return &publish.MyDashboardTodoRes{DashboardTodoModel: data}, nil
}
