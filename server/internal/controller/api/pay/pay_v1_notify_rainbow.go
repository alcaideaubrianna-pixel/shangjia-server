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

func (c *ControllerV1) NotifyRainbow(ctx context.Context, req *v1.NotifyRainbowReq) (res *v1.NotifyRainbowRes, err error) {
	if _, err = service.Pay().Notify(ctx, &payin.PayNotifyInp{PayType: consts.PayTypeRainbow}); err != nil {
		return nil, err
	}

	response.RText(g.RequestFromCtx(ctx), "success")
	return
}
