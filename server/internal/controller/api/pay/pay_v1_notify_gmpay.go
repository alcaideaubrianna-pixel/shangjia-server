package pay

import (
	"context"

	v1 "hotgo/api/api/pay/v1"
	"hotgo/internal/consts"
	"hotgo/internal/library/response"
	"hotgo/internal/model/input/payin"
	"hotgo/internal/service"

	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) NotifyGMPay(ctx context.Context, req *v1.NotifyGMPayReq) (res *v1.NotifyGMPayRes, err error) {
	if _, err = service.Pay().Notify(ctx, &payin.PayNotifyInp{PayType: consts.PayTypeGMPay}); err != nil {
		return nil, err
	}

	response.RText(g.RequestFromCtx(ctx), "success")
	return
}
