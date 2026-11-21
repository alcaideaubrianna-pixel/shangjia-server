package publish

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type CollectManageChannelListReq struct {
	g.Meta `path:"/publish/collect/manage/channel/list" method:"get" tags:"采集推送" summary:"采集频道列表"`
	sysin.CollectManageChannelListInp
}

type CollectManageChannelListRes struct {
	form.PageRes
	List []*sysin.CollectManageChannelModel `json:"list"`
}

type CollectManageChannelActionReq struct {
	g.Meta `path:"/publish/collect/manage/channel/action" method:"post" tags:"采集推送" summary:"操作采集频道"`
	sysin.CollectManageChannelActionInp
}

type CollectManageChannelActionRes struct{}
