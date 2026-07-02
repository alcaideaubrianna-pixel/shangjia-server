package publish

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type AdminSubmitTaskReq struct {
	g.Meta `path:"/publish/admin/task/submit" method:"post" tags:"上架插件管理端" summary:"提交上架任务"`
	sysin.TaskSubmitInp
}

type AdminSubmitTaskRes struct{}

type AdminPublishRecordListReq struct {
	g.Meta `path:"/publish/admin/record/list" method:"get" tags:"上架插件管理端" summary:"发送记录列表"`
	sysin.PublishRecordListInp
}

type AdminPublishRecordListRes struct {
	form.PageRes
	List []*sysin.PublishRecordModel `json:"list" dc:"发送记录列表"`
}

type MyPublishRecordListReq struct {
	g.Meta `path:"/publish/record/list" method:"get" tags:"上架插件" summary:"我的发送记录列表"`
	sysin.PublishRecordListInp
}

type MyPublishRecordListRes struct {
	form.PageRes
	List []*sysin.PublishRecordModel `json:"list" dc:"发送记录列表"`
}
