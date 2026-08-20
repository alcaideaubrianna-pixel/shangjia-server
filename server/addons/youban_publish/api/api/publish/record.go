package publish

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type AdminPublishRecordListReq struct {
	g.Meta `path:"/publish/admin/record/list" method:"get" tags:"上架插件管理端" summary:"发送记录列表"`
	sysin.PublishRecordListInp
}

type AdminPublishRecordListRes struct {
	form.PageRes
	List []*sysin.PublishRecordModel `json:"list" dc:"发送记录列表"`
}

type AdminPublishRecordClearReq struct {
	g.Meta `path:"/publish/admin/record/clear" method:"post" tags:"上架插件管理端" summary:"清空发送记录"`
	sysin.PublishRecordClearInp
}

type AdminPublishRecordClearRes struct{}

type AdminInclusionRecordListReq struct {
	g.Meta `path:"/publish/admin/profile/inclusion/list" method:"get" tags:"上架插件管理端" summary:"资料收录记录"`
	sysin.InclusionRecordListInp
}

type AdminInclusionRecordListRes struct {
	form.PageRes
	List []*sysin.InclusionRecordModel `json:"list"`
}

type AdminTgObserveQueueListReq struct {
	g.Meta `path:"/publish/admin/observe/queue/list" method:"get" tags:"上架插件管理端" summary:"TG队列观测统计"`
	sysin.TgObserveQueueListInp
}

type AdminTgObserveQueueListRes struct {
	form.PageRes
	List []*sysin.TgObserveQueueStatModel `json:"list" dc:"队列统计列表"`
}

type AdminTgObserveChannelListReq struct {
	g.Meta `path:"/publish/admin/observe/channel/list" method:"get" tags:"上架插件管理端" summary:"TG频道观测统计"`
	sysin.TgObserveChannelListInp
}

type AdminTgObserveChannelListRes struct {
	form.PageRes
	List []*sysin.TgObserveChannelStatModel `json:"list" dc:"频道统计列表"`
}

type AdminTgObserveBotListReq struct {
	g.Meta `path:"/publish/admin/observe/bot/list" method:"get" tags:"上架插件管理端" summary:"TG Bot观测统计"`
	sysin.TgObserveBotListInp
}

type AdminTgObserveBotListRes struct {
	form.PageRes
	List []*sysin.TgObserveBotStatModel `json:"list" dc:"Bot统计列表"`
}

type AdminDevPublishChainTestReq struct {
	g.Meta `path:"/publish/admin/dev/publishChainTest" method:"post" tags:"上架插件管理端" summary:"开发环境推送链路测试"`
	sysin.DevPublishChainTestInp
}

type AdminDevPublishChainTestRes struct {
	*sysin.DevPublishChainTestModel
}

type MyPublishRecordListReq struct {
	g.Meta `path:"/publish/record/list" method:"get" tags:"上架插件" summary:"我的发送记录列表"`
	sysin.PublishRecordListInp
}

type MyPublishRecordListRes struct {
	form.PageRes
	List []*sysin.PublishRecordModel `json:"list" dc:"发送记录列表"`
}

type MyPublishRecordClearReq struct {
	g.Meta `path:"/publish/record/clear" method:"post" tags:"上架插件" summary:"清空我的发送记录"`
	sysin.PublishRecordClearInp
}

type MyPublishRecordClearRes struct{}

type MyInclusionRecordListReq struct {
	g.Meta `path:"/publish/profile/inclusion/list" method:"get" tags:"上架插件" summary:"我的资料收录记录"`
	sysin.InclusionRecordListInp
}

type MyInclusionRecordListRes struct {
	form.PageRes
	List []*sysin.InclusionRecordModel `json:"list"`
}

type MyDevPublishChainTestReq struct {
	g.Meta `path:"/publish/dev/publishChainTest" method:"post" tags:"上架插件" summary:"开发环境我的推送链路测试"`
	sysin.DevPublishChainTestInp
}

type MyDevPublishChainTestRes struct {
	*sysin.DevPublishChainTestModel
}
