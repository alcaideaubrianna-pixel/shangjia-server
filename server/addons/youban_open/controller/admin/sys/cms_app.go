package sys

import (
	"context"
	"hotgo/addons/youban_open/api/admin/cms_app"
	"hotgo/addons/youban_open/service"
)

var CmsApp = cCmsApp{}

type cCmsApp struct{}

func (c *cCmsApp) List(ctx context.Context, req *cms_app.ListReq) (*cms_app.ListRes, error) {
	v, e := service.OpenAccess().AppList(ctx, &req.CmsAppListInp)
	if e != nil {
		return nil, e
	}
	return &cms_app.ListRes{List: v}, nil
}
func (c *cCmsApp) Save(ctx context.Context, req *cms_app.SaveReq) (*cms_app.SaveRes, error) {
	v, e := service.OpenAccess().AppSave(ctx, &req.CmsAppSaveInp)
	if e != nil {
		return nil, e
	}
	return &cms_app.SaveRes{CmsAppCredentialModel: v}, nil
}
func (c *cCmsApp) ResetSecret(ctx context.Context, req *cms_app.ResetSecretReq) (*cms_app.ResetSecretRes, error) {
	v, e := service.OpenAccess().AppResetSecret(ctx, &req.CmsAppResetSecretInp)
	if e != nil {
		return nil, e
	}
	return &cms_app.ResetSecretRes{CmsAppCredentialModel: v}, nil
}
