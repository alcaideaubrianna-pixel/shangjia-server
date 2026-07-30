package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/adminin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type NoticeListReq struct {
	g.Meta `path:"/publish/notice/list" method:"get" tags:"上架插件通知公告" summary:"通知公告列表"`
	sysin.NoticeListInp
}

type NoticeListRes struct {
	form.PageRes
	List []*adminin.NoticeListModel `json:"list" dc:"通知公告列表"`
}

type NoticeViewReq struct {
	g.Meta `path:"/publish/notice/view" method:"get" tags:"上架插件通知公告" summary:"通知公告详情"`
	sysin.NoticeViewInp
}

type NoticeViewRes struct {
	*adminin.NoticeViewModel
}

type NoticeEditReq struct {
	g.Meta `path:"/publish/notice/edit" method:"post" tags:"上架插件通知公告" summary:"保存通知公告"`
	sysin.NoticeEditInp
}

type NoticeEditRes struct{}

type NoticeDeleteReq struct {
	g.Meta `path:"/publish/notice/delete" method:"post" tags:"上架插件通知公告" summary:"删除通知公告"`
	sysin.NoticeDeleteInp
}

type NoticeDeleteRes struct{}

type NoticeMaxSortReq struct {
	g.Meta `path:"/publish/notice/maxSort" method:"get" tags:"上架插件通知公告" summary:"获取通知公告最大排序"`
	sysin.NoticeMaxSortInp
}

type NoticeMaxSortRes struct {
	*adminin.NoticeMaxSortModel
}

type NoticeStatusReq struct {
	g.Meta `path:"/publish/notice/status" method:"post" tags:"上架插件通知公告" summary:"更新通知公告状态"`
	sysin.NoticeStatusInp
}

type NoticeStatusRes struct{}

type NoticeEditNotifyReq struct {
	g.Meta `path:"/publish/notice/editNotify" method:"post" tags:"上架插件通知公告" summary:"发送通知"`
	sysin.NoticeEditInp
}

type NoticeEditNotifyRes struct{}

type NoticeEditNoticeReq struct {
	g.Meta `path:"/publish/notice/editNotice" method:"post" tags:"上架插件通知公告" summary:"发送公告"`
	sysin.NoticeEditInp
}

type NoticeEditNoticeRes struct{}

type NoticeEditLetterReq struct {
	g.Meta `path:"/publish/notice/editLetter" method:"post" tags:"上架插件通知公告" summary:"发送私信"`
	sysin.NoticeEditInp
}

type NoticeEditLetterRes struct{}

type PullMessagesReq struct {
	g.Meta `path:"/publish/notice/pullMessages" method:"get" tags:"上架插件通知公告" summary:"拉取通知公告"`
	sysin.PullMessagesInp
}

type PullMessagesRes struct {
	*adminin.PullMessagesModel
}

type UpReadReq struct {
	g.Meta `path:"/publish/notice/upRead" method:"post" tags:"上架插件通知公告" summary:"标记通知公告已读"`
	sysin.NoticeUpReadInp
}

type UpReadRes struct{}

type ReadAllReq struct {
	g.Meta `path:"/publish/notice/readAll" method:"post" tags:"上架插件通知公告" summary:"全部通知公告已读"`
	sysin.NoticeReadAllInp
}

type ReadAllRes struct{}

type MessageListReq struct {
	g.Meta `path:"/publish/notice/messageList" method:"get" tags:"上架插件通知公告" summary:"通知公告消息列表"`
	sysin.NoticeMessageListInp
}

type MessageListRes struct {
	List []*adminin.NoticeMessageListModel `json:"list" dc:"消息列表"`
	form.PageRes
}
