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

func (c *cPublish) CollectConfig(ctx context.Context, req *publish.CollectConfigReq) (res *publish.CollectConfigRes, err error) {
	data, err := service.SysPublish().CollectConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &publish.CollectConfigRes{CollectConfigModel: data}, nil
}

func (c *cPublish) CollectConfigSave(ctx context.Context, req *publish.CollectConfigSaveReq) (res *publish.CollectConfigSaveRes, err error) {
	if err = service.SysPublish().CollectConfigSave(ctx, &req.CollectConfigSaveInp); err != nil {
		return nil, err
	}
	return &publish.CollectConfigSaveRes{}, nil
}

func (c *cPublish) CollectStats(ctx context.Context, req *publish.CollectStatsReq) (res *publish.CollectStatsRes, err error) {
	data, err := service.SysPublish().CollectStats(ctx)
	if err != nil {
		return nil, err
	}
	return &publish.CollectStatsRes{CollectStatsModel: data}, nil
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
	data, err := service.SysPublish().CollectSourceDown(ctx, &req.CollectSourceDownInp)
	if err != nil {
		return nil, err
	}
	return &publish.CollectSourceDownRes{CollectSourceDownModel: data}, nil
}

func (c *cPublish) CollectSourceHistoryStart(ctx context.Context, req *publish.CollectSourceHistoryStartReq) (res *publish.CollectSourceHistoryStartRes, err error) {
	data, err := service.SysPublish().CollectSourceHistoryStart(ctx, &req.CollectSourceHistoryStartInp)
	if err != nil {
		return nil, err
	}
	return &publish.CollectSourceHistoryStartRes{CollectHistoryTaskModel: data}, nil
}

func (c *cPublish) CollectSourceReset(ctx context.Context, req *publish.CollectSourceResetReq) (res *publish.CollectSourceResetRes, err error) {
	data, err := service.SysPublish().CollectSourceReset(ctx, &req.CollectSourceResetInp)
	if err != nil {
		return nil, err
	}
	return &publish.CollectSourceResetRes{CollectSourceResetModel: data}, nil
}

func (c *cPublish) CollectHistoryTaskList(ctx context.Context, req *publish.CollectHistoryTaskListReq) (res *publish.CollectHistoryTaskListRes, err error) {
	list, totalCount, err := service.SysPublish().CollectHistoryTaskList(ctx, &req.CollectHistoryTaskListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.CollectHistoryTaskModel{}
	}
	res = new(publish.CollectHistoryTaskListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
}

func (c *cPublish) CollectHistoryLogList(ctx context.Context, req *publish.CollectHistoryLogListReq) (res *publish.CollectHistoryLogListRes, err error) {
	list, totalCount, err := service.SysPublish().CollectHistoryLogList(ctx, &req.CollectHistoryLogListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.CollectHistoryLogModel{}
	}
	res = new(publish.CollectHistoryLogListRes)
	res.List = list
	res.PageRes.Pack(req, totalCount)
	return
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

func (c *cPublish) CollectEventLogList(ctx context.Context, req *publish.CollectEventLogListReq) (res *publish.CollectEventLogListRes, err error) {
	list, totalCount, err := service.SysPublish().CollectEventLogList(ctx, &req.CollectEventLogListInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.CollectEventLogModel{}
	}
	res = new(publish.CollectEventLogListRes)
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

func (c *cPublish) CollectEventReprocess(ctx context.Context, req *publish.CollectEventReprocessReq) (res *publish.CollectEventReprocessRes, err error) {
	data, err := service.SysPublish().CollectEventReprocess(ctx, &req.CollectEventReprocessInp)
	if err != nil {
		return nil, err
	}
	return &publish.CollectEventReprocessRes{CollectEventReprocessModel: data}, nil
}

func (c *cPublish) CollectMaterialDiagnose(ctx context.Context, req *publish.CollectMaterialDiagnoseReq) (res *publish.CollectMaterialDiagnoseRes, err error) {
	data, err := service.SysPublish().CollectMaterialDiagnose(ctx, &req.CollectMaterialDiagnoseInp)
	if err != nil {
		return nil, err
	}
	return &publish.CollectMaterialDiagnoseRes{CollectMaterialDiagnoseModel: data}, nil
}

func (c *cPublish) CollectMediaBenchmark(ctx context.Context, req *publish.CollectMediaBenchmarkReq) (res *publish.CollectMediaBenchmarkRes, err error) {
	data, err := service.SysPublish().CollectMediaBenchmark(ctx, &req.CollectMediaBenchmarkInp)
	if err != nil {
		return nil, err
	}
	return &publish.CollectMediaBenchmarkRes{CollectMediaBenchmarkModel: data}, nil
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
	data, err := service.SysPublish().CollectReviewList(ctx, &req.CollectReviewListInp)
	if err != nil {
		return nil, err
	}
	if data == nil {
		data = &sysin.CollectReviewPageModel{}
	}
	if data.List == nil {
		data.List = []*sysin.CollectReviewModel{}
	}
	return &publish.CollectReviewListRes{CollectReviewPageModel: data}, nil
}

func (c *cPublish) CollectReviewAction(ctx context.Context, req *publish.CollectReviewActionReq) (res *publish.CollectReviewActionRes, err error) {
	if err = service.SysPublish().CollectReviewAction(ctx, &req.CollectReviewActionInp); err != nil {
		return nil, err
	}
	return &publish.CollectReviewActionRes{}, nil
}

func (c *cPublish) CollectReviewEdit(ctx context.Context, req *publish.CollectReviewEditReq) (res *publish.CollectReviewEditRes, err error) {
	if err = service.SysPublish().CollectReviewEdit(ctx, &req.CollectReviewEditInp); err != nil {
		return nil, err
	}
	return &publish.CollectReviewEditRes{}, nil
}

func (c *cPublish) CollectReviewDelete(ctx context.Context, req *publish.CollectReviewDeleteReq) (res *publish.CollectReviewDeleteRes, err error) {
	if err = service.SysPublish().CollectReviewDelete(ctx, &req.IdsInp); err != nil {
		return nil, err
	}
	return &publish.CollectReviewDeleteRes{}, nil
}
