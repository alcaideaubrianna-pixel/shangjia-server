// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package pay

import (
	"context"

	"hotgo/api/api/pay/v1"
)

type IPayV1 interface {
	NotifyAliPay(ctx context.Context, req *v1.NotifyAliPayReq) (res *v1.NotifyAliPayRes, err error)
	NotifyWxPay(ctx context.Context, req *v1.NotifyWxPayReq) (res *v1.NotifyWxPayRes, err error)
	NotifyQQPay(ctx context.Context, req *v1.NotifyQQPayReq) (res *v1.NotifyQQPayRes, err error)
	NotifyGMPay(ctx context.Context, req *v1.NotifyGMPayReq) (res *v1.NotifyGMPayRes, err error)
	NotifyRainbow(ctx context.Context, req *v1.NotifyRainbowReq) (res *v1.NotifyRainbowRes, err error)
}
