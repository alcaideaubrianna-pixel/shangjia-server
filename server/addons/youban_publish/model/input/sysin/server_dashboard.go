package sysin

type ServerDashboardModel struct {
	Stats             []*ServerDashboardStat              `json:"stats" dc:"核心指标"`
	TaskTrend         []*ServerDashboardTrendPoint        `json:"taskTrend" dc:"任务趋势"`
	ProfileTrend      []*ServerDashboardProfileTrendPoint `json:"profileTrend" dc:"资料上架趋势"`
	Health            []*ServerDashboardHealth            `json:"health" dc:"系统健康"`
	Todos             []*ServerDashboardTodo              `json:"todos" dc:"待处理事项"`
	PublishFailureTop []*ServerDashboardRank              `json:"publishFailureTop" dc:"发布失败Top10"`
	ProfilePublishTop []*ServerDashboardRank              `json:"profilePublishTop" dc:"资料发布Top10"`
	StartDate         string                              `json:"startDate" dc:"统计开始日期"`
	EndDate           string                              `json:"endDate" dc:"统计结束日期"`
	UpdatedAt         string                              `json:"updatedAt" dc:"更新时间"`
}

type ServerDashboardProfileTrendPoint struct {
	Date      string `json:"date" dc:"日期"`
	Created   int    `json:"created" dc:"新增资料"`
	Published int    `json:"published" dc:"上架资料"`
	Down      int    `json:"down" dc:"下架资料"`
}

type ServerDashboardStat struct {
	Key    string  `json:"key" dc:"指标键"`
	Title  string  `json:"title" dc:"标题"`
	Value  int     `json:"value" dc:"数值"`
	Suffix string  `json:"suffix" dc:"单位"`
	Rate   float64 `json:"rate" dc:"比例"`
}

type ServerDashboardTrendPoint struct {
	Date      string `json:"date" dc:"日期"`
	Created   int    `json:"created" dc:"新增任务"`
	Published int    `json:"published" dc:"发布成功"`
	Failed    int    `json:"failed" dc:"发布失败"`
	Canceled  int    `json:"canceled" dc:"取消任务"`
}

type ServerDashboardHealth struct {
	Key     string `json:"key" dc:"健康项键"`
	Title   string `json:"title" dc:"标题"`
	Status  string `json:"status" dc:"状态"`
	Value   string `json:"value" dc:"显示值"`
	Message string `json:"message" dc:"说明"`
}

type ServerDashboardTodo struct {
	Key       string `json:"key" dc:"唯一键"`
	Title     string `json:"title" dc:"标题"`
	Desc      string `json:"desc" dc:"描述"`
	Status    string `json:"status" dc:"状态"`
	UpdatedAt string `json:"updatedAt" dc:"更新时间"`
}

type ServerDashboardRank struct {
	Key    string `json:"key" dc:"唯一键"`
	Name   string `json:"name" dc:"名称"`
	Value  int    `json:"value" dc:"数值"`
	Desc   string `json:"desc" dc:"说明"`
	Status string `json:"status" dc:"状态"`
}
