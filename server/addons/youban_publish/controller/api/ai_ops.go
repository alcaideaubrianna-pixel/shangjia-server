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

func (c *cAIOps) ProfileRepublish(ctx context.Context, req *aiops.ProfileRepublishReq) (*aiops.ProfileRepublishRes, error) {
	result, err := service.SysPublish().AIOpsRepublishProfiles(ctx, &sysin.ProfileStatusInp{Ids: req.ProfileIds, Status: 1})
	if err != nil {
		return nil, err
	}
	return &aiops.ProfileRepublishRes{Message: result.Message}, nil
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
