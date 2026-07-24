package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublish) TenantVipStatus(ctx context.Context, req *publish.TenantVipStatusReq) (res *publish.TenantVipStatusRes, err error) {
	data, err := service.SysPublish().TenantVipStatus(ctx)
	if err != nil {
		return nil, err
	}
	res = &publish.TenantVipStatusRes{TenantVipStatusModel: data}
	return
}

func (c *cPublish) TenantVipPlans(ctx context.Context, req *publish.TenantVipPlansReq) (res *publish.TenantVipPlansRes, err error) {
	list, err := service.SysPublish().TenantVipPlans(ctx)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TenantVipPlanModel{}
	}
	res = &publish.TenantVipPlansRes{List: list}
	return
}

func (c *cPublish) TenantVipOrderCreate(ctx context.Context, req *publish.TenantVipOrderCreateReq) (res *publish.TenantVipOrderCreateRes, err error) {
	data, err := service.SysPublish().TenantVipOrderCreate(ctx, &req.TenantVipOrderCreateInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TenantVipOrderCreateRes{TenantVipOrderModel: data}
	return
}

func (c *cPublish) TenantVipOrderList(ctx context.Context, req *publish.TenantVipOrderListReq) (res *publish.TenantVipOrderListRes, err error) {
	list, totalCount, err := service.SysPublish().TenantVipOrderList(ctx, &req.TenantVipOrderListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.TenantVipOrderModel{}
	}
	res = new(publish.TenantVipOrderListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) TenantVipOrderPay(ctx context.Context, req *publish.TenantVipOrderPayReq) (res *publish.TenantVipOrderPayRes, err error) {
	data, err := service.SysPublish().TenantVipOrderPay(ctx, &req.TenantVipOrderPayInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TenantVipOrderPayRes{TenantVipOrderModel: data}
	return
}

func (c *cPublish) TenantVipCouponCheck(ctx context.Context, req *publish.TenantVipCouponCheckReq) (res *publish.TenantVipCouponCheckRes, err error) {
	data, err := service.SysPublish().TenantVipCouponCheck(ctx, &req.TenantVipCouponCheckInp)
	if err != nil {
		return nil, err
	}
	res = &publish.TenantVipCouponCheckRes{TenantVipCouponCheckModel: data}
	return
}

func (c *cPublish) MediaSimilarCount(ctx context.Context, req *publish.MediaSimilarCountReq) (res *publish.MediaSimilarCountRes, err error) {
	data, err := service.SysPublish().MediaSimilarCount(ctx, &req.MediaSimilarCountInp)
	if err != nil {
		return nil, err
	}
	res = &publish.MediaSimilarCountRes{MediaSimilarCountModel: data}
	return
}

func (c *cPublish) MediaSimilarList(ctx context.Context, req *publish.MediaSimilarListReq) (res *publish.MediaSimilarListRes, err error) {
	data, totalCount, err := service.SysPublish().MediaSimilarList(ctx, &req.MediaSimilarListInp)
	if err != nil {
		return nil, err
	}
	if data == nil {
		data = &sysin.MediaSimilarListModel{List: []*sysin.NoteModel{}}
	}
	if data.List == nil {
		data.List = []*sysin.NoteModel{}
	}
	res = new(publish.MediaSimilarListRes)
	res.MediaId = data.MediaId
	res.List = data.List
	res.PageRes.Pack(req, totalCount)
	return
}
