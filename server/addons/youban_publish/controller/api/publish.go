package api

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

var Publish = cPublish{}

type cPublish struct{}

func (c *cPublish) CurrentAccount(ctx context.Context, req *publish.CurrentAccountReq) (res *publish.CurrentAccountRes, err error) {
	data, err := service.SysPublish().CurrentAccount(ctx)
	if err != nil {
		return
	}
	res = &publish.CurrentAccountRes{CurrentAccountModel: data}
	return
}

func (c *cPublish) MyTaskList(ctx context.Context, req *publish.MyTaskListReq) (res *publish.MyTaskListRes, err error) {
	list, totalCount, err := service.SysPublish().MyTaskList(ctx, &req.TaskListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.TaskModel{}
	}
	res = new(publish.MyTaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) SaveTask(ctx context.Context, req *publish.SaveTaskReq) (res *publish.SaveTaskRes, err error) {
	id, err := service.SysPublish().MyTaskSave(ctx, &req.TaskSaveInp)
	if err != nil {
		return
	}
	res = &publish.SaveTaskRes{Id: id}
	return
}

func (c *cPublish) SubmitTask(ctx context.Context, req *publish.SubmitTaskReq) (res *publish.SubmitTaskRes, err error) {
	err = service.SysPublish().MyTaskSubmit(ctx, &req.TaskSubmitInp)
	if err != nil {
		return
	}
	res = &publish.SubmitTaskRes{}
	return
}

func (c *cPublish) CancelTask(ctx context.Context, req *publish.CancelTaskReq) (res *publish.CancelTaskRes, err error) {
	err = service.SysPublish().MyTaskCancel(ctx, &req.TaskCancelInp)
	if err != nil {
		return
	}
	res = &publish.CancelTaskRes{}
	return
}

func (c *cPublish) UploadMedia(ctx context.Context, req *publish.UploadMediaReq) (res *publish.UploadMediaRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("file")
	if file == nil {
		return nil, gerror.New("没有找到上传的文件")
	}
	data, err := service.SysPublish().MyMediaUpload(ctx, &req.MediaUploadInp, file)
	if err != nil {
		return nil, err
	}
	res = &publish.UploadMediaRes{MediaModel: data}
	return
}

func (c *cPublish) MediaList(ctx context.Context, req *publish.MediaListReq) (res *publish.MediaListRes, err error) {
	list, err := service.SysPublish().MyMediaList(ctx, &req.MediaListInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MediaListRes{List: list}
	return
}

func (c *cPublish) DeleteMedia(ctx context.Context, req *publish.DeleteMediaReq) (res *publish.DeleteMediaRes, err error) {
	if err = service.SysPublish().MyMediaDelete(ctx, &req.MediaDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.DeleteMediaRes{}
	return
}

func (c *cPublish) TelegramLoginStart(ctx context.Context, req *publish.TelegramLoginStartReq) (res *publish.TelegramLoginStartRes, err error) {
	data, err := service.SysPublish().TelegramLoginStart(ctx, &req.TelegramLoginStartInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TelegramLoginStartRes{TelegramLoginModel: data}
	return
}

func (c *cPublish) TelegramLoginStatus(ctx context.Context, req *publish.TelegramLoginStatusReq) (res *publish.TelegramLoginStatusRes, err error) {
	data, err := service.SysPublish().TelegramLoginStatus(ctx, &req.TelegramLoginStatusInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TelegramLoginStatusRes{TelegramLoginModel: data}
	return
}
