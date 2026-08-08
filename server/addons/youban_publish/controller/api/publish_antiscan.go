package api

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishAdmin) CloudResourceConfigView(ctx context.Context, req *publish.AdminCloudResourceConfigViewReq) (res *publish.AdminCloudResourceConfigViewRes, err error) {
	data, err := service.SysConfig().CloudResourceConfigView(ctx, &req.CloudResourceConfigViewInp)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminCloudResourceConfigViewRes{CloudResourceConfigViewModel: data}
	return
}

func (c *cPublishAdmin) CloudResourceConfigSave(ctx context.Context, req *publish.AdminCloudResourceConfigSaveReq) (res *publish.AdminCloudResourceConfigSaveRes, err error) {
	if err = service.SysConfig().CloudResourceConfigSave(ctx, &req.CloudResourceConfigSaveInp); err != nil {
		return nil, err
	}
	res = &publish.AdminCloudResourceConfigSaveRes{}
	return
}

func (c *cPublishAdmin) AntiScanPreview(ctx context.Context, req *publish.AdminAntiScanPreviewReq) (res *publish.AdminAntiScanPreviewRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("image")
	data, err := service.SysPublish().AdminAntiScanPreview(ctx, &req.AntiScanPreviewInp, file)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAntiScanPreviewRes{AntiScanPreviewModel: data}
	return
}

func (c *cPublishAdmin) AntiScanConfigSaveTab(ctx context.Context, req *publish.AdminAntiScanConfigSaveTabReq) (res *publish.AdminAntiScanConfigSaveTabRes, err error) {
	if err = service.SysConfig().AntiScanConfigSaveTab(ctx, &req.AntiScanConfigSaveTabInp); err != nil {
		return nil, err
	}
	res = &publish.AdminAntiScanConfigSaveTabRes{}
	return
}

func (c *cPublishAdmin) AntiScanMaterialList(ctx context.Context, req *publish.AdminAntiScanMaterialListReq) (res *publish.AdminAntiScanMaterialListRes, err error) {
	list, err := service.SysPublish().AdminAntiScanMaterialList(ctx, &req.AntiScanMaterialListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.AntiScanMaterialModel{}
	}
	res = &publish.AdminAntiScanMaterialListRes{List: list}
	return
}

func (c *cPublishAdmin) AntiScanMaterialUpload(ctx context.Context, req *publish.AdminAntiScanMaterialUploadReq) (res *publish.AdminAntiScanMaterialUploadRes, err error) {
	file := g.RequestFromCtx(ctx).GetUploadFile("file")
	data, err := service.SysPublish().AdminAntiScanMaterialUpload(ctx, &req.AntiScanMaterialUploadInp, file)
	if err != nil {
		return nil, err
	}
	res = &publish.AdminAntiScanMaterialUploadRes{AntiScanMaterialModel: data}
	return
}

func (c *cPublishAdmin) AntiScanMaterialDelete(ctx context.Context, req *publish.AdminAntiScanMaterialDeleteReq) (res *publish.AdminAntiScanMaterialDeleteRes, err error) {
	if err = service.SysPublish().AdminAntiScanMaterialDelete(ctx, &req.AntiScanMaterialDeleteInp); err != nil {
		return nil, err
	}
	res = &publish.AdminAntiScanMaterialDeleteRes{}
	return
}
