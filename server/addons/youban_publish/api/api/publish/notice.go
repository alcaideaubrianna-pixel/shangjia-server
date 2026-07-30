package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/adminin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

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
