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

type UploadMediaReq struct {
	g.Meta `path:"/publish/media/upload" method:"post" mime:"multipart/form-data" tags:"上架插件" summary:"上传任务媒体"`
	sysin.MediaUploadInp
}

type UploadMediaRes struct {
	*sysin.MediaModel
}

type MediaListReq struct {
	g.Meta `path:"/publish/media/list" method:"get" tags:"上架插件" summary:"任务媒体列表"`
	sysin.MediaListInp
}

type MediaListRes struct {
	List []*sysin.MediaModel `json:"list" dc:"媒体列表"`
}

type DeleteMediaReq struct {
	g.Meta `path:"/publish/media/delete" method:"post" tags:"上架插件" summary:"删除任务媒体"`
	sysin.MediaDeleteInp
}

type DeleteMediaRes struct{}

type TelegramLoginStartReq struct {
	g.Meta `path:"/telegram/login/start" method:"post" tags:"上架插件" summary:"发起Telegram扫码登录"`
	sysin.TelegramLoginStartInp
}

type TelegramLoginStartRes struct {
	*sysin.TelegramLoginModel
}

type TelegramLoginStatusReq struct {
	g.Meta `path:"/telegram/login/status" method:"get" tags:"上架插件" summary:"查询Telegram扫码登录状态"`
	sysin.TelegramLoginStatusInp
}

type TelegramLoginStatusRes struct {
	*sysin.TelegramLoginModel
}

type TelegramLoginPasswordReq struct {
	g.Meta `path:"/telegram/login/password" method:"post" tags:"上架插件" summary:"提交Telegram二次验证密码"`
	sysin.TelegramLoginPasswordInp
}

type TelegramLoginPasswordRes struct {
	*sysin.TelegramLoginModel
}
