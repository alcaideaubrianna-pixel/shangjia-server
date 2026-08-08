package sys

import (
	"context"
	"hotgo/addons/youban_two_way_bot/api/admin/cooperation"
	"hotgo/addons/youban_two_way_bot/service"
)

var Cooperation = cCooperation{}

type cCooperation struct{}

func (c *cCooperation) ConfigView(ctx context.Context, req *cooperation.ConfigViewReq) (*cooperation.ConfigViewRes, error) {
	v, e := service.SysTwoWayBot().AdminCooperationConfigView(ctx)
	return &cooperation.ConfigViewRes{CooperationConfigModel: v}, e
}
func (c *cCooperation) ConfigSave(ctx context.Context, req *cooperation.ConfigSaveReq) (*cooperation.ConfigSaveRes, error) {
	v, e := service.SysTwoWayBot().AdminCooperationConfigSave(ctx, &req.CooperationConfigSaveInp)
	return &cooperation.ConfigSaveRes{CooperationConfigModel: v}, e
}
func (c *cCooperation) ApplicationList(ctx context.Context, req *cooperation.ApplicationListReq) (*cooperation.ApplicationListRes, error) {
	l, n, e := service.SysTwoWayBot().AdminCooperationApplicationList(ctx, &req.CooperationApplicationListInp)
	r := &cooperation.ApplicationListRes{List: l}
	r.PageRes.Pack(req, n)
	return r, e
}
func (c *cCooperation) ApplicationApprove(ctx context.Context, req *cooperation.ApplicationApproveReq) (*cooperation.ApplicationApproveRes, error) {
	return &cooperation.ApplicationApproveRes{}, service.SysTwoWayBot().AdminCooperationApplicationApprove(ctx, &req.CooperationApplicationActionInp)
}
func (c *cCooperation) ApplicationReject(ctx context.Context, req *cooperation.ApplicationRejectReq) (*cooperation.ApplicationRejectRes, error) {
	return &cooperation.ApplicationRejectRes{}, service.SysTwoWayBot().AdminCooperationApplicationReject(ctx, &req.CooperationApplicationActionInp)
}
func (c *cCooperation) ApplicationCancel(ctx context.Context, req *cooperation.ApplicationCancelReq) (*cooperation.ApplicationCancelRes, error) {
	return &cooperation.ApplicationCancelRes{}, service.SysTwoWayBot().AdminCooperationApplicationCancel(ctx, &req.CooperationApplicationActionInp)
}
func (c *cCooperation) ApplicationTerminate(ctx context.Context, req *cooperation.ApplicationTerminateReq) (*cooperation.ApplicationTerminateRes, error) {
	return &cooperation.ApplicationTerminateRes{}, service.SysTwoWayBot().AdminCooperationApplicationTerminate(ctx, &req.CooperationApplicationActionInp)
}
func (c *cCooperation) ApplicationRetry(ctx context.Context, req *cooperation.ApplicationRetryReq) (*cooperation.ApplicationRetryRes, error) {
	return &cooperation.ApplicationRetryRes{}, service.SysTwoWayBot().AdminCooperationApplicationRetry(ctx, &req.CooperationApplicationActionInp)
}
func (c *cCooperation) ApplicationBlacklist(ctx context.Context, req *cooperation.ApplicationBlacklistReq) (*cooperation.ApplicationBlacklistRes, error) {
	return &cooperation.ApplicationBlacklistRes{}, service.SysTwoWayBot().AdminCooperationApplicationBlacklist(ctx, &req.CooperationApplicationActionInp)
}
func (c *cCooperation) ApplicationUnblacklist(ctx context.Context, req *cooperation.ApplicationUnblacklistReq) (*cooperation.ApplicationUnblacklistRes, error) {
	return &cooperation.ApplicationUnblacklistRes{}, service.SysTwoWayBot().AdminCooperationApplicationUnblacklist(ctx, &req.CooperationApplicationActionInp)
}
func (c *cCooperation) Import(ctx context.Context, req *cooperation.ImportReq) (*cooperation.ImportRes, error) {
	v, e := service.SysTwoWayBot().AdminCooperationImport(ctx, &req.CooperationImportInp)
	return &cooperation.ImportRes{CooperationImportModel: v}, e
}
