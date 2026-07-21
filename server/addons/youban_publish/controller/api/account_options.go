package api

import (
	"context"

	"hotgo/addons/youban_publish/api/api/publish"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func (c *cPublishAdmin) AccountOptions(ctx context.Context, req *publish.AdminAccountOptionsReq) (res *publish.AdminAccountOptionsRes, err error) {
	list, err := service.SysPublish().AdminAccountOptions(ctx, &req.AccountOptionsInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.AccountOptionModel{}
	}
	res = &publish.AdminAccountOptionsRes{List: list}
	return
}
