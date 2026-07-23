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
		jumpDriver: epay.New(config),
	}
}

type rainbowPay struct {
	jumpDriver payDriver
}

type payDriver interface {
	Refund(ctx context.Context, in payin.RefundInp) (res *payin.RefundModel, err error)
	Notify(ctx context.Context, in payin.NotifyInp) (res *payin.NotifyModel, err error)
	CreateOrder(ctx context.Context, in payin.CreateOrderInp) (res *payin.CreateOrderModel, err error)
}

func (h *rainbowPay) Refund(ctx context.Context, in payin.RefundInp) (res *payin.RefundModel, err error) {
	return h.jumpDriver.Refund(ctx, in)
}

func (h *rainbowPay) Notify(ctx context.Context, in payin.NotifyInp) (res *payin.NotifyModel, err error) {
	return h.jumpDriver.Notify(ctx, in)
}

func (h *rainbowPay) CreateOrder(ctx context.Context, in payin.CreateOrderInp) (res *payin.CreateOrderModel, err error) {
	return h.jumpDriver.CreateOrder(ctx, in)
}
