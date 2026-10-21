package sys

import (
	"context"
	"hotgo/addons/youban_publish/api/admin/publish"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishServer) AccountLoginLink(ctx context.Context, req *publish.AccountLoginLinkReq) (*publish.AccountLoginLinkRes, error) {
	res, err := service.SysPublish().ServerAccountLoginLink(ctx, &req.AccountLoginLinkInp)
	if err != nil {
		return nil, err
	}
	return &publish.AccountLoginLinkRes{AccountLoginModel: res}, nil
}
