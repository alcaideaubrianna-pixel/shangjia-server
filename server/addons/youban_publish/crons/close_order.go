package crons

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cron"
	"hotgo/internal/service"
)

const tenantVipOrderCloseTimeout = 10 * time.Minute

func init() {
	cron.Register(VipOrderClose)
}

// VipOrderClose 关闭超过 10 分钟未支付的上架 VIP 订单。
var VipOrderClose = &cVipOrderClose{name: "youbanPublishVipCloseOrder"}

type cVipOrderClose struct {
	name string
}

func (c *cVipOrderClose) GetName() string {
	return c.name
}

func (c *cVipOrderClose) Execute(ctx context.Context, parser *cron.Parser) (err error) {
	cols := dao.AdminOrder.Columns()
	_, err = service.AdminOrder().Model(ctx).
		Where(cols.OrderType, "youban_tenant_vip").
		Where(cols.Status, consts.OrderStatusNotPay).
		WhereLTE(cols.CreatedAt, gtime.Now().Add(-tenantVipOrderCloseTimeout)).
		Data(g.Map{
			cols.Status:    consts.OrderStatusClose,
			cols.UpdatedAt: gtime.Now(),
		}).Update()
	if err != nil {
		parser.Logger.Warningf(ctx, "cron VipOrderClose Execute err:%+v", err)
	}
	return
}
