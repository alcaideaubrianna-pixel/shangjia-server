package sysin

import (
	"context"
	"github.com/gogf/gf/v2/errors/gerror"
)

type AccountLoginLinkInp struct {
	Id int64 `json:"id" dc:"目标账号ID"`
}

func (in *AccountLoginLinkInp) Filter(ctx context.Context) error {
	if in == nil || in.Id <= 0 {
		return gerror.New("请选择目标账号")
	}
	return nil
}
