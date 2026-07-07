package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublish) CollectSourceList(ctx context.Context, req *publish.CollectSourceListReq) (res *publish.CollectSourceListRes, err error) {
	list, totalCount, err := service.SysPublish().CollectSourceList(ctx, &req.CollectSourceListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.CollectSourceModel{}
	}
	res = new(publish.CollectSourceListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) CollectSourceSave(ctx context.Context, req *publish.CollectSourceSaveReq) (res *publish.CollectSourceSaveRes, err error) {
	id, err := service.SysPublish().CollectSourceSave(ctx, &req.CollectSourceSaveInp)
	if err != nil {
		return nil, err
	}
	return &publish.CollectSourceSaveRes{Id: id}, nil
}

func (c *cPublish) CollectSourceDelete(ctx context.Context, req *publish.CollectSourceDeleteReq) (res *publish.CollectSourceDeleteRes, err error) {
	if err = service.SysPublish().CollectSourceDelete(ctx, &req.IdsInp); err != nil {
		return nil, err
	}
	return &publish.CollectSourceDeleteRes{}, nil
}

func (c *cPublish) CollectSourceStatus(ctx context.Context, req *publish.CollectSourceStatusReq) (res *publish.CollectSourceStatusRes, err error) {
	if err = service.SysPublish().CollectSourceStatus(ctx, &req.CollectStatusInp); err != nil {
		return nil, err
	}
	return &publish.CollectSourceStatusRes{}, nil
}

func (c *cPublish) CollectSourceDown(ctx context.Context, req *publish.CollectSourceDownReq) (res *publish.CollectSourceDownRes, err error) {
	if err = service.SysPublish().CollectSourceDown(ctx, &req.CollectSourceDownInp); err != nil {
		return nil, err
	}
	return &publish.CollectSourceDownRes{}, nil
}

func (c *cPublish) CollectRuleList(ctx context.Context, req *publish.CollectRuleListReq) (res *publish.CollectRuleListRes, err error) {
	list, totalCount, err := service.SysPublish().CollectRuleList(ctx, &req.CollectRuleListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.CollectRuleModel{}
	}
	res = new(publish.CollectRuleListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) CollectRuleSave(ctx context.Context, req *publish.CollectRuleSaveReq) (res *publish.CollectRuleSaveRes, err error) {
	id, err := service.SysPublish().CollectRuleSave(ctx, &req.CollectRuleSaveInp)
	if err != nil {
		return nil, err
	}
	return &publish.CollectRuleSaveRes{Id: id}, nil
}

func (c *cPublish) CollectRuleDelete(ctx context.Context, req *publish.CollectRuleDeleteReq) (res *publish.CollectRuleDeleteRes, err error) {
	if err = service.SysPublish().CollectRuleDelete(ctx, &req.IdsInp); err != nil {
		return nil, err
	}
	return &publish.CollectRuleDeleteRes{}, nil
}

func (c *cPublish) CollectEventList(ctx context.Context, req *publish.CollectEventListReq) (res *publish.CollectEventListRes, err error) {
	list, totalCount, err := service.SysPublish().CollectEventList(ctx, &req.CollectEventListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.CollectEventModel{}
	}
	res = new(publish.CollectEventListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) CollectEventClear(ctx context.Context, req *publish.CollectEventClearReq) (res *publish.CollectEventClearRes, err error) {
	if err = service.SysPublish().CollectEventClear(ctx, &req.CollectEventClearInp); err != nil {
		return nil, err
	}
	return &publish.CollectEventClearRes{}, nil
}

func (c *cPublish) CollectEventProcess(ctx context.Context, req *publish.CollectEventProcessReq) (res *publish.CollectEventProcessRes, err error) {
	if err = service.SysPublish().CollectEventProcess(ctx, &req.CollectEventProcessInp); err != nil {
		return nil, err
	}
	return &publish.CollectEventProcessRes{}, nil
}

func (c *cPublish) CollectContentList(ctx context.Context, req *publish.CollectContentListReq) (res *publish.CollectContentListRes, err error) {
	list, totalCount, err := service.SysPublish().CollectContentList(ctx, &req.CollectContentListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.CollectContentModel{}
	}
	res = new(publish.CollectContentListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) CollectContentView(ctx context.Context, req *publish.CollectContentViewReq) (res *publish.CollectContentViewRes, err error) {
	data, err := service.SysPublish().CollectContentView(ctx, &req.CollectContentViewInp)
	if err != nil {
		return nil, err
	}
	return &publish.CollectContentViewRes{CollectContentModel: data}, nil
}

func (c *cPublish) CollectReviewList(ctx context.Context, req *publish.CollectReviewListReq) (res *publish.CollectReviewListRes, err error) {
	list, totalCount, err := service.SysPublish().CollectReviewList(ctx, &req.CollectReviewListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.CollectReviewModel{}
	}
	res = new(publish.CollectReviewListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) CollectReviewAction(ctx context.Context, req *publish.CollectReviewActionReq) (res *publish.CollectReviewActionRes, err error) {
	if err = service.SysPublish().CollectReviewAction(ctx, &req.CollectReviewActionInp); err != nil {
		return nil, err
	}
	return &publish.CollectReviewActionRes{}, nil
}
