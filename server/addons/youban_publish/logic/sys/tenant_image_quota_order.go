package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	baseentity "hotgo/internal/model/entity"
	"hotgo/internal/model/input/payin"
	"hotgo/internal/service"
)

func (s *sSysPublish) tenantImageQuotaOrderCreate(ctx context.Context, in *sysin.TenantVipOrderCreateInp) (*sysin.TenantVipOrderModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in.Quantity <= 0 || in.Quantity > 1000 {
		return nil, gerror.New("图片额度购买金额需在1到1000 USDT之间")
	}
	cfg, err := service.SysConfig().GetYoubanPublishVip(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil || !cfg.Enabled {
		return nil, gerror.New("图片额度购买暂未开放")
	}
	payItem := tenantVipDefaultPayItem(cfg)
	if strings.TrimSpace(in.PayType) == "" && payItem != nil {
		in.PayType = payItem.PayType
	}
	if strings.TrimSpace(in.TradeType) == "" && payItem != nil {
		in.TradeType = payItem.TradeType
	}
	amount := float64(in.Quantity)
	quotaCount := in.Quantity * imageQuotaPerUSDT
	orderSn := fmt.Sprintf("YBPIMG%d%d", account.TenantId, time.Now().UnixNano())
	subject := fmt.Sprintf("图片处理额度:%d张", quotaCount)
	now := gtime.Now()
	var res *sysin.TenantVipOrderModel
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		orderColumns := dao.AdminOrder.Columns()
		result, err := tx.Model(dao.AdminOrder.Table()).Ctx(ctx).Fields(
			orderColumns.MemberId, orderColumns.OrderType, orderColumns.ProductId,
			orderColumns.OrderSn, orderColumns.Money, orderColumns.Remark,
			orderColumns.Status, orderColumns.CreatedAt, orderColumns.UpdatedAt,
		).Data(baseentity.AdminOrder{
			MemberId: contexts.GetUserId(ctx), OrderType: tenantVipOrderType,
			ProductId: account.TenantId, OrderSn: orderSn, Money: amount,
			Remark: subject, Status: consts.OrderStatusNotPay, CreatedAt: now, UpdatedAt: now,
		}).Insert()
		if err != nil {
			return gerror.Wrap(err, "创建图片额度订单失败")
		}
		orderID, _ := result.LastInsertId()
		created, err := service.Pay().Create(ctx, payin.PayCreateInp{
			Subject: subject,
			Detail: gjson.New(g.Map{
				"productType": imageQuotaProductType, "quantity": in.Quantity,
				"quotaCount": quotaCount, "tenantId": account.TenantId,
			}),
			OrderSn: orderSn, OrderGroup: tenantVipOrderGroup,
			PayType: in.PayType, TradeType: in.TradeType, PayAmount: amount,
			ReturnUrl: tenantVipPaymentReturnURL(in.ReturnUrl, orderSn),
		})
		if err != nil {
			return err
		}
		res = tenantVipOrderModel(&baseentity.AdminOrder{
			Id: orderID, OrderSn: orderSn, Money: amount,
			Status: consts.OrderStatusNotPay, CreatedAt: now,
		}, nil, nil)
		res.ProductType = imageQuotaProductType
		res.Quantity = quotaCount
		res.PlanCode = imageQuotaProductType
		res.PlanName = "图片处理额度"
		res.Currency = "USDT"
		res.Order = created.Order
		if created.Order != nil {
			res.PayUrl = created.Order.PayURL
			res.TradeType = created.Order.TradeType
		}
		return nil
	})
	return res, err
}
