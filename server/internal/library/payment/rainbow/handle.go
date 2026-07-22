// Package rainbow 彩虹易支付
package rainbow

import (
	"context"

	"hotgo/internal/library/payment/epay"
	"hotgo/internal/model"
	"hotgo/internal/model/input/payin"
)

func New(config *model.PayConfig) *rainbowPay {
	return &rainbowPay{
		driver: epay.New(config),
	}
}

type rainbowPay struct {
	driver *epay.EPay
}

func (h *rainbowPay) Refund(ctx context.Context, in payin.RefundInp) (res *payin.RefundModel, err error) {
	return h.driver.Refund(ctx, in)
}

func (h *rainbowPay) Notify(ctx context.Context, in payin.NotifyInp) (res *payin.NotifyModel, err error) {
	return h.driver.Notify(ctx, in)
}

func (h *rainbowPay) CreateOrder(ctx context.Context, in payin.CreateOrderInp) (res *payin.CreateOrderModel, err error) {
	return h.driver.CreateOrder(ctx, in)
}
