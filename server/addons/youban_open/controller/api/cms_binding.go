package api

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_open/api/api/publish"
	"hotgo/addons/youban_open/service"
	"hotgo/internal/library/publishtenant"
)

var CmsBinding = cCmsBinding{}

type cCmsBinding struct{}

func (c *cCmsBinding) Claim(ctx context.Context, req *publish.CmsBindingClaimReq) (*publish.CmsBindingClaimRes, error) {
	tenantID, err := publishtenant.CurrentID(ctx)
	if err != nil {
		return nil, gerror.Wrap(err, "读取当前租户失败")
	}
	data, err := service.OpenAccess().ClaimBinding(ctx, tenantID, &req.CmsBindingClaimInp)
	if err != nil {
		return nil, err
	}
	return &publish.CmsBindingClaimRes{CmsBindingModel: data}, nil
}
func (c *cCmsBinding) Mine(ctx context.Context, _ *publish.CmsBindingMineReq) (*publish.CmsBindingMineRes, error) {
	tenantID, err := publishtenant.CurrentID(ctx)
	if err != nil {
		return nil, gerror.Wrap(err, "读取当前租户失败")
	}
	list, err := service.OpenAccess().TenantBinding(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return &publish.CmsBindingMineRes{List: list}, nil
}
func (c *cCmsBinding) Lookup(ctx context.Context, req *publish.CmsBindingLookupReq) (*publish.CmsBindingLookupRes, error) {
	data, err := service.OpenAccess().LookupBindingCode(ctx, req.Code)
	if err != nil {
		return nil, err
	}
	return &publish.CmsBindingLookupRes{CmsBindingLookupModel: data}, nil
}

func (c *cCmsBinding) Revoke(ctx context.Context, req *publish.CmsBindingRevokeReq) (*publish.CmsBindingRevokeRes, error) {
	tenantID, err := publishtenant.CurrentID(ctx)
	if err != nil {
		return nil, gerror.Wrap(err, "读取当前租户失败")
	}
	data, err := service.OpenAccess().RevokeTenantBinding(ctx, tenantID, &req.CmsBindingRevokeInp)
	if err != nil {
		return nil, err
	}
	return &publish.CmsBindingRevokeRes{CmsBindingModel: data}, nil
}
