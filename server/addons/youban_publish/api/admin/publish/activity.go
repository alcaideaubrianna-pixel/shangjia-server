package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type ActivityListReq struct {
	g.Meta `path:"/publish/activity/list" method:"get" tags:"上架插件后台" summary:"活动列表"`
}

type ActivityListRes struct {
	List []*sysin.ActivityModel `json:"list" dc:"活动列表"`
}

type ActivitySaveReq struct {
	g.Meta `path:"/publish/activity/save" method:"post" tags:"上架插件后台" summary:"保存活动配置"`
	sysin.ActivitySaveInp
}

type ActivitySaveRes struct{}

type ActivityRewardListReq struct {
	g.Meta `path:"/publish/activity/reward/list" method:"get" tags:"上架插件后台" summary:"活动奖励记录"`
	sysin.ActivityRewardListInp
}

type ActivityRewardListRes struct {
	form.PageRes
	List []*sysin.ActivityRewardModel `json:"list" dc:"奖励记录"`
}

type ActivityUserStatusReq struct {
	g.Meta `path:"/publish/activity/user/status" method:"get" tags:"上架插件后台" summary:"用户活动状态"`
	sysin.ActivityUserStatusInp
}

type ActivityUserStatusRes struct {
	List []*sysin.ActivityUserStatusModel `json:"list" dc:"用户活动状态"`
}

type ActivityDebugReq struct {
	g.Meta `path:"/publish/activity/debug" method:"post" tags:"上架插件后台" summary:"调试用户活动"`
	sysin.ActivityDebugInp
}

type ActivityDebugRes struct {
	*sysin.ActivityUserStatusModel
}

type ActivityResetReq struct {
	g.Meta `path:"/publish/activity/reset" method:"post" tags:"上架插件后台" summary:"重置用户活动"`
	sysin.ActivityResetInp
}

type ActivityResetRes struct{}
