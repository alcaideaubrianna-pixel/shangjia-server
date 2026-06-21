package sys

import (
	"context"
	"hotgo/api/admin/contentnote"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

var ContentNote = cContentNote{}

type cContentNote struct{}

// List 查看内容笔记列表。
func (c *cContentNote) List(ctx context.Context, req *contentnote.ListReq) (res *contentnote.ListRes, err error) {
	list, totalCount, err := service.SysContentNote().List(ctx, &req.ContentNoteListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ContentNoteListModel{}
	}
	res = new(contentnote.ListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

// View 获取内容笔记详情。
func (c *cContentNote) View(ctx context.Context, req *contentnote.ViewReq) (res *contentnote.ViewRes, err error) {
	data, err := service.SysContentNote().View(ctx, &req.ContentNoteViewInp)
	if err != nil {
		return
	}
	res = new(contentnote.ViewRes)
	res.ContentNoteViewModel = data
	return
}

func (c *cContentNote) Edit(ctx context.Context, req *contentnote.EditReq) (res *contentnote.EditRes, err error) {
	err = service.SysContentNote().Edit(ctx, &req.ContentNoteEditInp)
	if err != nil {
		return
	}
	res = new(contentnote.EditRes)
	return
}

func (c *cContentNote) MediaEdit(ctx context.Context, req *contentnote.MediaEditReq) (res *contentnote.MediaEditRes, err error) {
	err = service.SysContentNote().MediaEdit(ctx, &req.ContentNoteMediaEditInp)
	if err != nil {
		return
	}
	res = new(contentnote.MediaEditRes)
	return
}

func (c *cContentNote) BatchDelete(ctx context.Context, req *contentnote.BatchDeleteReq) (res *contentnote.BatchDeleteRes, err error) {
	err = service.SysContentNote().BatchDelete(ctx, &req.ContentNoteBatchDeleteInp)
	if err != nil {
		return
	}
	res = new(contentnote.BatchDeleteRes)
	return
}

func (c *cContentNote) BatchReview(ctx context.Context, req *contentnote.BatchReviewReq) (res *contentnote.BatchReviewRes, err error) {
	err = service.SysContentNote().BatchReview(ctx, &req.ContentNoteBatchReviewInp)
	if err != nil {
		return
	}
	res = new(contentnote.BatchReviewRes)
	return
}

func (c *cContentNote) BatchStatus(ctx context.Context, req *contentnote.BatchStatusReq) (res *contentnote.BatchStatusRes, err error) {
	err = service.SysContentNote().BatchStatus(ctx, &req.ContentNoteBatchStatusInp)
	if err != nil {
		return
	}
	res = new(contentnote.BatchStatusRes)
	return
}

func (c *cContentNote) BatchRemark(ctx context.Context, req *contentnote.BatchRemarkReq) (res *contentnote.BatchRemarkRes, err error) {
	err = service.SysContentNote().BatchRemark(ctx, &req.ContentNoteBatchRemarkInp)
	if err != nil {
		return
	}
	res = new(contentnote.BatchRemarkRes)
	return
}
