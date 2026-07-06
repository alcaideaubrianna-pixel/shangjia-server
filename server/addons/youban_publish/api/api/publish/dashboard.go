package publish

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type AdminDashboardOverviewReq struct {
	g.Meta `path:"/publish/admin/dashboard/overview" method:"get" tags:"上架插件管理端" summary:"管理端工作台概览"`
}

type AdminDashboardOverviewRes struct {
	*sysin.DashboardOverviewModel
}

type AdminDashboardTrendReq struct {
	g.Meta `path:"/publish/admin/dashboard/trend" method:"get" tags:"上架插件管理端" summary:"管理端工作台趋势"`
	sysin.TrendInp
}

type AdminDashboardTrendRes struct {
	*sysin.DashboardTrendModel
}

type AdminDashboardTodoReq struct {
	g.Meta `path:"/publish/admin/dashboard/todo" method:"get" tags:"上架插件管理端" summary:"管理端工作台待办"`
}

type AdminDashboardTodoRes struct {
	*sysin.DashboardTodoModel
}

type AdminDashboardRankReq struct {
	g.Meta `path:"/publish/admin/dashboard/rank" method:"get" tags:"上架插件管理端" summary:"管理端工作台排行"`
}

type AdminDashboardRankRes struct {
	*sysin.DashboardRankModel
}

type MyDashboardOverviewReq struct {
	g.Meta `path:"/publish/dashboard/overview" method:"get" tags:"上架插件" summary:"上架端工作台概览"`
}

type MyDashboardOverviewRes struct {
	*sysin.DashboardOverviewModel
}

type MyDashboardTrendReq struct {
	g.Meta `path:"/publish/dashboard/trend" method:"get" tags:"上架插件" summary:"上架端工作台趋势"`
	sysin.TrendInp
}

type MyDashboardTrendRes struct {
	*sysin.DashboardTrendModel
}

type MyDashboardTodoReq struct {
	g.Meta `path:"/publish/dashboard/todo" method:"get" tags:"上架插件" summary:"上架端工作台待办"`
}

type MyDashboardTodoRes struct {
	*sysin.DashboardTodoModel
}
