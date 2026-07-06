package api

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublish) AccountProfileView(ctx context.Context, req *publish.AccountProfileViewReq) (res *publish.AccountProfileViewRes, err error) {
	data, err := service.SysPublish().AccountProfileView(ctx, &req.AccountProfileViewInp)
	if err != nil {
		return nil, err
	}
	return &publish.AccountProfileViewRes{AccountProfileModel: data}, nil
}

func (c *cPublish) AccountProfileSave(ctx context.Context, req *publish.AccountProfileSaveReq) (res *publish.AccountProfileSaveRes, err error) {
	data, err := service.SysPublish().AccountProfileSave(ctx, &req.AccountProfileSaveInp)
	if err != nil {
		return nil, err
	}
	return &publish.AccountProfileSaveRes{AccountProfileModel: data}, nil
}

func (c *cPublish) AccountFollowList(ctx context.Context, req *publish.AccountFollowListReq) (res *publish.AccountFollowListRes, err error) {
	list, totalCount, err := service.SysPublish().AccountFollowList(ctx, &req.AccountFollowListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.AccountFollowModel{}
	}
	res = new(publish.AccountFollowListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) AccountFollowApply(ctx context.Context, req *publish.AccountFollowApplyReq) (res *publish.AccountFollowApplyRes, err error) {
	if err = service.SysPublish().AccountFollowApply(ctx, &req.AccountFollowApplyInp); err != nil {
		return nil, err
	}
	return &publish.AccountFollowApplyRes{}, nil
}

func (c *cPublish) AccountFollowAction(ctx context.Context, req *publish.AccountFollowActionReq) (res *publish.AccountFollowActionRes, err error) {
	if err = service.SysPublish().AccountFollowAction(ctx, &req.AccountFollowActionInp); err != nil {
		return nil, err
	}
	return &publish.AccountFollowActionRes{}, nil
}

func (c *cPublish) FollowNoteList(ctx context.Context, req *publish.FollowNoteListReq) (res *publish.FollowNoteListRes, err error) {
	list, totalCount, err := service.SysPublish().FollowNoteList(ctx, &req.FollowNoteListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.NoteModel{}
	}
	res = new(publish.FollowNoteListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) FollowNoteImageSearch(ctx context.Context, req *publish.FollowNoteImageSearchReq) (res *publish.FollowNoteImageSearchRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("image")
	if file == nil {
		return nil, gerror.New("请先上传要搜索的图片")
	}
	list, totalCount, err := service.SysPublish().FollowNoteImageSearch(ctx, &req.FollowNoteListInp, file)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.NoteModel{}
	}
	res = new(publish.FollowNoteImageSearchRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}
