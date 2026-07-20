package sync

import (
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_feiniu_sync/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type TenantOptionsReq struct {
	g.Meta `path:"/sync/options/tenants" method:"get" tags:"FeiNiu同步" summary:"上架租户选项"`
	sysin.OptionListInp
}
type TenantOptionsRes struct {
	List []*sysin.TenantOptionModel `json:"list" dc:"租户选项"`
}
type AdminAccountOptionsReq struct {
	g.Meta `path:"/sync/options/adminAccounts" method:"get" tags:"FeiNiu同步" summary:"上架管理员账号选项"`
	sysin.OptionListInp
}
type AdminAccountOptionsRes struct {
	List []*sysin.AccountOptionModel `json:"list" dc:"账号选项"`
}
type AccountOptionsReq struct {
	g.Meta `path:"/sync/options/accounts" method:"get" tags:"FeiNiu同步" summary:"上架账号选项"`
	sysin.OptionListInp
}
type AccountOptionsRes struct {
	List []*sysin.AccountOptionModel `json:"list" dc:"账号选项"`
}

type ConfigListReq struct {
	g.Meta `path:"/sync/config/list" method:"get" tags:"FeiNiu同步" summary:"配置列表"`
	sysin.ConfigListInp
}
type ConfigListRes struct {
	form.PageRes
	List []*sysin.ConfigModel `json:"list" dc:"列表"`
}
type ConfigViewReq struct {
	g.Meta `path:"/sync/config/view" method:"get" tags:"FeiNiu同步" summary:"配置详情"`
	sysin.ConfigViewInp
}
type ConfigViewRes struct{ *sysin.ConfigModel }
type ConfigSaveReq struct {
	g.Meta `path:"/sync/config/save" method:"post" tags:"FeiNiu同步" summary:"保存配置"`
	sysin.ConfigSaveInp
}
type ConfigSaveRes struct{ *sysin.ConfigModel }
type ConfigDeleteReq struct {
	g.Meta `path:"/sync/config/delete" method:"post" tags:"FeiNiu同步" summary:"删除配置"`
	sysin.ConfigDeleteInp
}
type ConfigDeleteRes struct{}
type ConfigAutoSyncReq struct {
	g.Meta `path:"/sync/config/autoSync" method:"post" tags:"FeiNiu同步" summary:"启动暂停自动同步"`
	sysin.ConfigAutoSyncInp
}
type ConfigAutoSyncRes struct{}
type ConfigTestReq struct {
	g.Meta `path:"/sync/config/test" method:"post" tags:"FeiNiu同步" summary:"测试连接"`
	sysin.ConfigTestInp
}
type ConfigTestRes struct{ *sysin.ConfigTestModel }
type DashboardReq struct {
	g.Meta `path:"/sync/dashboard" method:"get" tags:"FeiNiu同步" summary:"同步概览"`
	sysin.DashboardInp
}
type DashboardRes struct{ *sysin.DashboardModel }
type DashboardSummaryReq struct {
	g.Meta `path:"/sync/dashboard/summary" method:"get" tags:"FeiNiu同步" summary:"监控概览"`
	sysin.DashboardInp
}
type DashboardSummaryRes struct{ *sysin.DashboardSummaryModel }
type DashboardTrendReq struct {
	g.Meta `path:"/sync/dashboard/trend" method:"get" tags:"FeiNiu同步" summary:"采集趋势"`
	sysin.DashboardInp
}
type DashboardTrendRes struct {
	List []*sysin.DashboardTrendModel `json:"list" dc:"趋势列表"`
}
type DashboardChannelRankReq struct {
	g.Meta `path:"/sync/dashboard/channelRank" method:"get" tags:"FeiNiu同步" summary:"频道排行"`
	sysin.DashboardInp
	Limit int `json:"limit" dc:"数量"`
}
type DashboardChannelRankRes struct {
	List []*sysin.DashboardChannelRankModel `json:"list" dc:"频道排行"`
}
type DashboardRecentRunsReq struct {
	g.Meta `path:"/sync/dashboard/recentRuns" method:"get" tags:"FeiNiu同步" summary:"最近运行"`
	sysin.DashboardInp
	Limit int `json:"limit" dc:"数量"`
}
type DashboardRecentRunsRes struct {
	List []*sysin.RunModel `json:"list" dc:"最近运行"`
}
type ChannelMapListReq struct {
	g.Meta `path:"/sync/channel/list" method:"get" tags:"FeiNiu同步" summary:"频道映射"`
	sysin.ChannelMapListInp
}
type ChannelMapListRes struct {
	form.PageRes
	List []*sysin.ChannelMapModel `json:"list" dc:"列表"`
}
type ChannelClearReq struct {
	g.Meta `path:"/sync/channel/clear" method:"post" tags:"FeiNiu同步" summary:"清空同步数据"`
	sysin.ChannelClearInp
}
type ChannelClearRes struct{ *sysin.ChannelClearModel }
type ChannelCopyReq struct {
	g.Meta `path:"/sync/channel/copy" method:"post" tags:"FeiNiu同步" summary:"复制频道资料"`
	sysin.ChannelCopyInp
}
type ChannelCopyRes struct{ *sysin.ChannelCopyModel }
type ChannelDisableReq struct {
	g.Meta `path:"/sync/channel/disable" method:"post" tags:"FeiNiu同步" summary:"停用频道账号"`
	sysin.ChannelDisableInp
}
type ChannelDisableRes struct{ *sysin.ChannelDisableModel }
type RunListReq struct {
	g.Meta `path:"/sync/run/list" method:"get" tags:"FeiNiu同步" summary:"运行记录"`
	sysin.RunListInp
}
type RunListRes struct {
	form.PageRes
	List []*sysin.RunModel `json:"list" dc:"列表"`
}
type RunViewReq struct {
	g.Meta `path:"/sync/run/view" method:"get" tags:"FeiNiu同步" summary:"运行详情"`
	sysin.RunViewInp
}
type RunViewRes struct{ *sysin.RunModel }
type RunItemListReq struct {
	g.Meta `path:"/sync/run/items" method:"get" tags:"FeiNiu同步" summary:"运行明细"`
	sysin.RunItemListInp
}
type RunItemListRes struct {
	form.PageRes
	List []*sysin.RunItemModel `json:"list" dc:"运行明细"`
}
type RunStartReq struct {
	g.Meta `path:"/sync/run/start" method:"post" tags:"FeiNiu同步" summary:"开始同步"`
	sysin.RunStartInp
}
type RunStartRes struct{ *sysin.RunStartModel }
