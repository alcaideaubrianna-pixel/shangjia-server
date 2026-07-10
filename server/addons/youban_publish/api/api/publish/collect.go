package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type CollectSourceListReq struct {
	g.Meta `path:"/publish/collect/source/list" method:"get" tags:"上架插件" summary:"采集源列表"`
	sysin.CollectSourceListInp
}

type CollectSourceListRes struct {
	form.PageRes
	List []*sysin.CollectSourceModel `json:"list" dc:"采集源列表"`
}

type CollectConfigReq struct {
	g.Meta `path:"/publish/collect/config" method:"get" tags:"上架插件" summary:"采集总开关配置"`
}

type CollectConfigRes struct {
	*sysin.CollectConfigModel
}

type CollectConfigSaveReq struct {
	g.Meta `path:"/publish/collect/config" method:"post" tags:"上架插件" summary:"保存采集总开关"`
	sysin.CollectConfigSaveInp
}

type CollectConfigSaveRes struct{}

type CollectStatsReq struct {
	g.Meta `path:"/publish/collect/stats" method:"get" tags:"上架插件" summary:"采集统计"`
}

type CollectStatsRes struct {
	*sysin.CollectStatsModel
}

type CollectSourceSaveReq struct {
	g.Meta `path:"/publish/collect/source/save" method:"post" tags:"上架插件" summary:"保存采集源"`
	sysin.CollectSourceSaveInp
}

type CollectSourceSaveRes struct {
	Id int64 `json:"id" dc:"采集源ID"`
}

type CollectSourceDeleteReq struct {
	g.Meta `path:"/publish/collect/source/delete" method:"post" tags:"上架插件" summary:"删除采集源"`
	sysin.IdsInp
}

type CollectSourceDeleteRes struct{}

type CollectSourceStatusReq struct {
	g.Meta `path:"/publish/collect/source/status" method:"post" tags:"上架插件" summary:"采集源开关"`
	sysin.CollectStatusInp
}

type CollectSourceStatusRes struct{}

type CollectSourceDownReq struct {
	g.Meta `path:"/publish/collect/source/down" method:"post" tags:"上架插件" summary:"一键下架采集源"`
	sysin.CollectSourceDownInp
}

type CollectSourceDownRes struct {
	*sysin.CollectSourceDownModel
}

type CollectSourceHistoryStartReq struct {
	g.Meta `path:"/publish/collect/source/history/start" method:"post" tags:"上架插件" summary:"启动采集源历史采集"`
	sysin.CollectSourceHistoryStartInp
}

type CollectSourceHistoryStartRes struct {
	*sysin.CollectHistoryTaskModel
}

type CollectSourceTriggerReq struct {
	g.Meta `path:"/publish/collect/source/trigger" method:"post" tags:"上架插件" summary:"手动触发采集源推送"`
	sysin.CollectSourceTriggerInp
}

type CollectSourceTriggerRes struct {
	*sysin.CollectSourceTriggerModel
}

type CollectSourceResetReq struct {
	g.Meta `path:"/publish/collect/source/reset" method:"post" tags:"上架插件" summary:"开发模式重置采集源推送状态"`
	sysin.CollectSourceResetInp
}

type CollectSourceResetRes struct {
	*sysin.CollectSourceResetModel
}

type CollectHistoryTaskListReq struct {
	g.Meta `path:"/publish/collect/history/task/list" method:"get" tags:"上架插件" summary:"历史采集任务列表"`
	sysin.CollectHistoryTaskListInp
}

type CollectHistoryTaskListRes struct {
	form.PageRes
	List []*sysin.CollectHistoryTaskModel `json:"list" dc:"历史采集任务列表"`
}

type CollectHistoryLogListReq struct {
	g.Meta `path:"/publish/collect/history/log/list" method:"get" tags:"上架插件" summary:"历史采集任务日志"`
	sysin.CollectHistoryLogListInp
}

type CollectHistoryLogListRes struct {
	form.PageRes
	List []*sysin.CollectHistoryLogModel `json:"list" dc:"历史采集任务日志"`
}

type CollectRuleListReq struct {
	g.Meta `path:"/publish/collect/rule/list" method:"get" tags:"上架插件" summary:"采集规则列表"`
	sysin.CollectRuleListInp
}

type CollectRuleListRes struct {
	form.PageRes
	List []*sysin.CollectRuleModel `json:"list" dc:"规则列表"`
}

type CollectRuleSaveReq struct {
	g.Meta `path:"/publish/collect/rule/save" method:"post" tags:"上架插件" summary:"保存采集规则"`
	sysin.CollectRuleSaveInp
}

type CollectRuleSaveRes struct {
	Id int64 `json:"id" dc:"规则ID"`
}

type CollectRuleDeleteReq struct {
	g.Meta `path:"/publish/collect/rule/delete" method:"post" tags:"上架插件" summary:"删除采集规则"`
	sysin.IdsInp
}

type CollectRuleDeleteRes struct{}

type CollectEventListReq struct {
	g.Meta `path:"/publish/collect/event/list" method:"get" tags:"上架插件" summary:"采集事件列表"`
	sysin.CollectEventListInp
}

type CollectEventListRes struct {
	form.PageRes
	List []*sysin.CollectEventModel `json:"list" dc:"事件列表"`
}

type CollectEventClearReq struct {
	g.Meta `path:"/publish/collect/event/clear" method:"post" tags:"上架插件" summary:"清空采集源事件"`
	sysin.CollectEventClearInp
}

type CollectEventClearRes struct{}

type CollectEventProcessReq struct {
	g.Meta `path:"/publish/collect/event/process" method:"post" tags:"上架插件" summary:"手动处理采集事件"`
	sysin.CollectEventProcessInp
}

type CollectEventProcessRes struct{}

type CollectContentListReq struct {
	g.Meta `path:"/publish/collect/content/list" method:"get" tags:"上架插件" summary:"采集内容池列表"`
	sysin.CollectContentListInp
}

type CollectContentListRes struct {
	form.PageRes
	List []*sysin.CollectContentModel `json:"list" dc:"内容池列表"`
}

type CollectContentViewReq struct {
	g.Meta `path:"/publish/collect/content/view" method:"get" tags:"上架插件" summary:"采集内容池详情"`
	sysin.CollectContentViewInp
}

type CollectContentViewRes struct {
	*sysin.CollectContentModel
}

type CollectReviewListReq struct {
	g.Meta `path:"/publish/collect/review/list" method:"get" tags:"上架插件" summary:"采集审核列表"`
	sysin.CollectReviewListInp
}

type CollectReviewListRes struct {
	form.PageRes
	List []*sysin.CollectReviewModel `json:"list" dc:"审核列表"`
}

type CollectReviewActionReq struct {
	g.Meta `path:"/publish/collect/review/action" method:"post" tags:"上架插件" summary:"采集审核操作"`
	sysin.CollectReviewActionInp
}

type CollectReviewActionRes struct{}
