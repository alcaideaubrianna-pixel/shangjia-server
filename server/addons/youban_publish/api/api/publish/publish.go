package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type CurrentAccountReq struct {
	g.Meta `path:"/publish/account/current" method:"get" tags:"上架插件" summary:"当前上架账号"`
}

type CurrentAccountRes struct {
	*sysin.CurrentAccountModel
}

type MyTaskListReq struct {
	g.Meta `path:"/publish/task/list" method:"get" tags:"上架插件" summary:"我的上架任务列表"`
	sysin.TaskListInp
}

type MyTaskListRes struct {
	form.PageRes
	List []*sysin.TaskModel `json:"list" dc:"任务列表"`
}

type SaveTaskReq struct {
	g.Meta `path:"/publish/task/save" method:"post" tags:"上架插件" summary:"保存我的上架任务"`
	sysin.TaskSaveInp
}

type SaveTaskRes struct {
	Id int64 `json:"id" dc:"任务ID"`
}

type SubmitTaskReq struct {
	g.Meta `path:"/publish/task/submit" method:"post" tags:"上架插件" summary:"提交我的上架任务"`
	sysin.TaskSubmitInp
}

type SubmitTaskRes struct{}

type CancelTaskReq struct {
	g.Meta `path:"/publish/task/cancel" method:"post" tags:"上架插件" summary:"取消我的上架任务"`
	sysin.TaskCancelInp
}

type CancelTaskRes struct{}
