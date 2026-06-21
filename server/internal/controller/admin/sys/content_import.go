package sys

import (
	"context"
	"hotgo/api/admin/contentimport"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

var ContentImport = cContentImport{}

type cContentImport struct{}

// RunFeiNiu 手动执行 FeiNiu 导入。
func (c *cContentImport) RunFeiNiu(ctx context.Context, req *contentimport.RunFeiNiuReq) (res *contentimport.RunFeiNiuRes, err error) {
	req.TriggerType = "manual"
	data, err := service.SysContent().ImportFeiNiu(ctx, &req.ContentImportFeiNiuInp)
	if err != nil {
		return
	}
	res = new(contentimport.RunFeiNiuRes)
	res.ContentImportFeiNiuModel = data
	return
}

// Overview 获取内容导入概览。
func (c *cContentImport) Overview(ctx context.Context, req *contentimport.OverviewReq) (res *contentimport.OverviewRes, err error) {
	data, err := service.SysContent().ImportOverview(ctx, &req.ContentImportOverviewInp)
	if err != nil {
		return
	}
	res = new(contentimport.OverviewRes)
	res.ContentImportOverviewModel = data
	return
}

// RunList 获取内容导入运行记录。
func (c *cContentImport) RunList(ctx context.Context, req *contentimport.RunListReq) (res *contentimport.RunListRes, err error) {
	list, totalCount, err := service.SysContent().ImportRunList(ctx, &req.ContentImportRunListInp)
	if err != nil {
		return
	}
	if list == nil {
		list = []*sysin.ContentImportRunListModel{}
	}
	res = new(contentimport.RunListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

// AutoSync 设置 FeiNiu 自动同步状态。
func (c *cContentImport) AutoSync(ctx context.Context, req *contentimport.AutoSyncReq) (res *contentimport.AutoSyncRes, err error) {
	data, err := service.SysContent().SetImportAutoSync(ctx, &req.ContentImportAutoSyncInp)
	if err != nil {
		return
	}
	res = new(contentimport.AutoSyncRes)
	res.ContentImportAutoSyncModel = data
	return
}

func (c *cContentImport) ReviewConfig(ctx context.Context, req *contentimport.ReviewConfigReq) (res *contentimport.ReviewConfigRes, err error) {
	data, err := service.SysContent().ImportReviewConfig(ctx, &req.ContentImportReviewConfigInp)
	if err != nil {
		return
	}
	res = new(contentimport.ReviewConfigRes)
	res.ContentImportReviewConfigModel = data
	return
}

func (c *cContentImport) SaveReviewConfig(ctx context.Context, req *contentimport.SaveReviewConfigReq) (res *contentimport.SaveReviewConfigRes, err error) {
	data, err := service.SysContent().SetImportReviewConfig(ctx, &req.ContentImportReviewConfigEditInp)
	if err != nil {
		return
	}
	res = new(contentimport.SaveReviewConfigRes)
	res.ContentImportReviewConfigModel = data
	return
}
