package sysin

type DashboardOverviewModel struct {
	Stats      []*DashboardStatModel      `json:"stats" dc:"核心指标"`
	Health     []*DashboardHealthModel    `json:"health" dc:"系统健康"`
	QuickLinks []*DashboardQuickLinkModel `json:"quickLinks" dc:"快捷入口"`
	Profile    *ProfileStatsModel         `json:"profile" dc:"资料统计"`
}

type DashboardTrendModel struct {
	ProfileTrend []*TrendPointModel     `json:"profileTrend" dc:"资料趋势"`
	PublishTrend []*DashboardTrendPoint `json:"publishTrend" dc:"发布趋势"`
}

type DashboardTodoModel struct {
	Items []*DashboardTodoItemModel `json:"items" dc:"待办列表"`
}

type DashboardRankModel struct {
	Publishers []*DashboardRankItemModel `json:"publishers" dc:"上架账号排行"`
	Channels   []*DashboardRankItemModel `json:"channels" dc:"频道排行"`
	Errors     []*DashboardRankItemModel `json:"errors" dc:"错误排行"`
}

type DashboardStatModel struct {
	Key    string  `json:"key" dc:"指标键"`
	Title  string  `json:"title" dc:"标题"`
	Value  int     `json:"value" dc:"数值"`
	Suffix string  `json:"suffix,omitempty" dc:"单位"`
	Rate   float64 `json:"rate,omitempty" dc:"百分比"`
}

type DashboardHealthModel struct {
	Key     string `json:"key" dc:"健康项键"`
	Title   string `json:"title" dc:"标题"`
	Status  string `json:"status" dc:"状态"`
	Value   string `json:"value" dc:"显示值"`
	Message string `json:"message" dc:"说明"`
}

type DashboardQuickLinkModel struct {
	Key   string `json:"key" dc:"快捷入口键"`
	Title string `json:"title" dc:"标题"`
	Path  string `json:"path" dc:"前端路径"`
	Badge int    `json:"badge" dc:"角标数量"`
}

type DashboardTodoItemModel struct {
	Key       string `json:"key" dc:"唯一键"`
	Type      string `json:"type" dc:"待办类型"`
	Title     string `json:"title" dc:"标题"`
	Desc      string `json:"desc" dc:"描述"`
	Status    string `json:"status" dc:"状态"`
	Path      string `json:"path" dc:"跳转路径"`
	UpdatedAt string `json:"updatedAt" dc:"更新时间"`
}

type DashboardTrendPoint struct {
	Date    string `json:"date" dc:"日期"`
	Success int    `json:"success" dc:"成功数"`
	Failed  int    `json:"failed" dc:"失败数"`
	Pending int    `json:"pending" dc:"待处理数"`
}

type DashboardRankItemModel struct {
	Key    string `json:"key" dc:"唯一键"`
	Name   string `json:"name" dc:"名称"`
	Value  int    `json:"value" dc:"数量"`
	Desc   string `json:"desc" dc:"说明"`
	Status string `json:"status" dc:"状态"`
}
