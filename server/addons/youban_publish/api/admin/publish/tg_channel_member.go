package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type ChannelMemberSyncStartReq struct {
	g.Meta `path:"/publish/admin/channel/member/sync/start" method:"post" tags:"上架插件后台" summary:"同步TG频道成员"`
	sysin.TgChannelMemberSyncStartInp
}

type ChannelMemberSyncStartRes struct {
	*sysin.TgChannelMemberSyncModel
}

type ChannelMemberSyncViewReq struct {
	g.Meta `path:"/publish/admin/channel/member/sync/view" method:"get" tags:"上架插件后台" summary:"TG频道成员同步进度"`
	sysin.TgChannelMemberSyncViewInp
}

type ChannelMemberSyncViewRes struct {
	*sysin.TgChannelMemberSyncModel
}

type ChannelMemberSyncCancelReq struct {
	g.Meta `path:"/publish/admin/channel/member/sync/cancel" method:"post" tags:"上架插件后台" summary:"取消TG频道成员同步"`
	sysin.TgChannelMemberSyncCancelInp
}

type ChannelMemberSyncCancelRes struct{}

type ChannelMemberListReq struct {
	g.Meta `path:"/publish/admin/channel/member/list" method:"get" tags:"上架插件后台" summary:"TG频道成员缓存列表"`
	sysin.TgChannelMemberListInp
}

type ChannelMemberListRes struct {
	form.PageRes
	List []*sysin.TgChannelMemberModel `json:"list" dc:"成员列表"`
}

type ChannelMemberExportReq struct {
	g.Meta `path:"/publish/admin/channel/member/export" method:"get" tags:"上架插件后台" summary:"导出TG频道成员缓存"`
	sysin.TgChannelMemberListInp
}

type ChannelMemberExportRes struct{}
