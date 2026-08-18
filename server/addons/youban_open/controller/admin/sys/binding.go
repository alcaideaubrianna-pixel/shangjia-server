package sys

import (
	"context"
	"hotgo/addons/youban_open/api/admin/binding"
	"hotgo/addons/youban_open/service"
)

var Binding = cBinding{}

type cBinding struct{}

func (c *cBinding) List(ctx context.Context, req *binding.ListReq) (*binding.ListRes, error) {
	list, err := service.OpenAccess().AdminBindings(ctx, &req.CmsBindingListInp)
	if err != nil {
		return nil, err
	}
	return &binding.ListRes{List: list}, nil
}
func (c *cBinding) Status(ctx context.Context, req *binding.StatusReq) (*binding.StatusRes, error) {
	data, err := service.OpenAccess().UpdateBinding(ctx, req.AppId, &req.CmsBindingStatusInp)
	if err != nil {
		return nil, err
	}
	return &binding.StatusRes{CmsBindingModel: data}, nil
}
