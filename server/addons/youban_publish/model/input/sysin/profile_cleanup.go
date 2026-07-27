package sysin

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
)

type ProfilePurgeDeletedInp struct {
	TenantId  int64 `json:"tenantId" v:"required|min:1#请选择账号归属|请选择账号归属" dc:"租户ID"`
	AccountId int64 `json:"accountId" v:"required|min:1#请选择账号|请选择账号" dc:"账号ID"`
}

func (in *ProfilePurgeDeletedInp) Filter(ctx context.Context) error {
	_ = ctx
	if in == nil || in.TenantId <= 0 {
		return gerror.New("请选择账号归属")
	}
	if in.AccountId <= 0 {
		return gerror.New("请选择账号")
	}
	return nil
}

type ProfilePurgeDeletedModel struct {
	DeletedCount int `json:"deletedCount" dc:"清理资料数量"`
}
