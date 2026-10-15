package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/aiops"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

var AIOps = cAIOps{}

type cAIOps struct{}

func (c *cAIOps) ProfileMedia(ctx context.Context, req *aiops.ProfileMediaReq) (*aiops.ProfileMediaRes, error) {
	result, err := service.SysPublish().AIOpsRebuildProfileMedia(ctx, req.ProfileIds, !req.Apply)
	if err != nil {
		return nil, err
	}
	return &aiops.ProfileMediaRes{Candidates: result.Candidates, Recoverable: result.Recoverable, Requeued: result.Requeued, ProfileIds: result.ProfileIDs}, nil
}

func (c *cAIOps) ProfileMediaQueue(ctx context.Context, req *aiops.ProfileMediaQueueReq) (*aiops.ProfileMediaQueueRes, error) {
	ids, err := service.SysPublish().AIOpsQueueBotMediaRepair(ctx, req.ProfileIds, req.Limit)
	if err != nil {
		return nil, err
	}
	return &aiops.ProfileMediaQueueRes{Queued: len(ids), ProfileIds: ids}, nil
}

func (c *cAIOps) ProfileRepublish(ctx context.Context, req *aiops.ProfileRepublishReq) (*aiops.ProfileRepublishRes, error) {
	result, err := service.SysPublish().AIOpsRepublishProfiles(ctx, &sysin.ProfileStatusInp{Ids: req.ProfileIds, Status: 1})
	if err != nil {
		return nil, err
	}
	return &aiops.ProfileRepublishRes{Message: result.Message}, nil
}

func (c *cAIOps) ProfileOffline(ctx context.Context, req *aiops.ProfileOfflineReq) (*aiops.ProfileOfflineRes, error) {
	result, err := service.SysPublish().AIOpsOfflineProfiles(ctx, req.ProfileIds)
	if err != nil {
		return nil, err
	}
	return &aiops.ProfileOfflineRes{Message: result.Message}, nil
}

func (c *cAIOps) ProfileDelete(ctx context.Context, req *aiops.ProfileDeleteReq) (*aiops.ProfileDeleteRes, error) {
	ids, err := service.SysPublish().AIOpsDeleteImportedProfiles(ctx, req.TenantId, req.AccountId, req.ProfileIds, !req.Apply)
	if err != nil {
		return nil, err
	}
	res := &aiops.ProfileDeleteRes{Candidates: len(ids), ProfileIds: ids}
	if req.Apply {
		res.Deleted = len(ids)
	}
	return res, nil
}

func (c *cAIOps) TelegramMessageContext(ctx context.Context, req *aiops.TelegramMessageContextReq) (*aiops.TelegramMessageContextRes, error) {
	result, err := service.SysPublish().AIOpsTelegramMessageContext(ctx, &req.AIOpsTelegramMessageContextInp)
	if err != nil {
		return nil, err
	}
	return &aiops.TelegramMessageContextRes{AIOpsTelegramMessageContextModel: result}, nil
}

func (c *cAIOps) TelegramMessageOffline(ctx context.Context, req *aiops.TelegramMessageOfflineReq) (*aiops.TelegramMessageOfflineRes, error) {
	result, err := service.SysPublish().AIOpsTelegramMessageOffline(ctx, &req.AIOpsTelegramOfflineInp)
	if err != nil {
		return nil, err
	}
	return &aiops.TelegramMessageOfflineRes{AIOpsTelegramOfflineResult: result}, nil
}

func (c *cAIOps) RepairPlan(ctx context.Context, req *aiops.RepairPlanReq) (*aiops.RepairPlanRes, error) {
	result, err := service.SysPublish().AIOpsCreateRepairPlan(ctx, &req.AIOpsRepairPlanInp)
	if err != nil {
		return nil, err
	}
	return &aiops.RepairPlanRes{AIOpsRepairPlanModel: result}, nil
}

func (c *cAIOps) RepairApply(ctx context.Context, req *aiops.RepairApplyReq) (*aiops.RepairApplyRes, error) {
	result, err := service.SysPublish().AIOpsApplyRepairPlan(ctx, &req.AIOpsRepairApplyInp)
	if err != nil {
		return nil, err
	}
	return &aiops.RepairApplyRes{AIOpsRepairApplyModel: result}, nil
}
