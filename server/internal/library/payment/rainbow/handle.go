// Package rainbow 彩虹易支付
package rainbow

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"hotgo/internal/model"
	"hotgo/internal/model/input/payin"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
)

const (
	defaultGateway = "https://pay.v8jisu.cn"
	signTypeMD5    = "MD5"
)

func New(config *model.PayConfig) *rainbowPay {
	return &rainbowPay{
		config: config,
	}
}

type rainbowPay struct {
	config *model.PayConfig
}

func (h *rainbowPay) Refund(ctx context.Context, in payin.RefundInp) (res *payin.RefundModel, err error) {
	err = gerror.New("彩虹易支付暂未接入退款，如有疑问请联系管理员")
	return
}

func (h *rainbowPay) Notify(ctx context.Context, in payin.NotifyInp) (res *payin.NotifyModel, err error) {
	if err = h.validateConfig(); err != nil {
		return
	}

	request := ghttp.RequestFromCtx(ctx)
	params := request.GetMap()
	signParamsMap := make(map[string]string, len(params))
	for key, value := range params {
		signParamsMap[key] = gconv.String(value)
	}
	g.Log().Info(ctx, "收到彩虹易支付回调", signParamsMap)

	if err = verifyParams(signParamsMap, h.config.RainbowKey); err != nil {
		return
	}

	var notify *notifyRequest
	if err = gconv.Scan(signParamsMap, &notify); err != nil {
		return
	}
	if notify == nil {
		err = gerror.New("解析彩虹易支付回调参数失败")
		return
	}
	if notify.TradeStatus != "TRADE_SUCCESS" {
		err = gerror.New("非交易支付成功状态，无需处理")
		return
	}
	if notify.OutTradeNo == "" {
		err = gerror.New("彩虹易支付回调缺少商户订单号")
		return
	}

	payAt := gtime.Now()
	if notify.Endtime != "" {
		payAt = gtime.NewFromStr(notify.Endtime)
	}

	res = &payin.NotifyModel{
		OutTradeNo:    notify.OutTradeNo,
		TransactionId: firstNonEmpty(notify.ApiTradeNo, notify.TradeNo),
		PayAt:         payAt,
		ActualAmount:  gconv.Float64(notify.Money),
	}
	return
}

func (h *rainbowPay) CreateOrder(ctx context.Context, in payin.CreateOrderInp) (res *payin.CreateOrderModel, err error) {
	if err = h.validateConfig(); err != nil {
		return
	}
	if in.Pay == nil {
		err = gerror.New("支付订单不能为空")
		return
	}

	tradeType := strings.TrimSpace(in.Pay.TradeType)

	params := map[string]string{
		"pid":          strings.TrimSpace(h.config.RainbowPid),
		"out_trade_no": in.Pay.OutTradeNo,
		"notify_url":   in.Pay.NotifyUrl,
		"return_url":   in.Pay.ReturnUrl,
		"name":         in.Pay.Subject,
		"money":        fmt.Sprintf("%.2f", in.Pay.PayAmount),
		"sign_type":    signTypeMD5,
	}
	if tradeType != "" {
		params["type"] = tradeType
	}

	params["sign"] = signParams(params, h.config.RainbowKey)

	res = &payin.CreateOrderModel{
		TradeType:  tradeType,
		PayURL:     h.submitURL(params),
		OutTradeNo: in.Pay.OutTradeNo,
	}
	return
}

func (h *rainbowPay) submitURL(params map[string]string) string {
	query := url.Values{}
	for key, value := range params {
		if value != "" {
			query.Set(key, value)
		}
	}
	return h.gateway() + "/submit.php?" + query.Encode()
}

func (h *rainbowPay) validateConfig() error {
	if h.config == nil {
		return gerror.New("支付配置未初始化")
	}
	if strings.TrimSpace(h.config.RainbowPid) == "" {
		return gerror.New("请先配置彩虹易支付商户ID")
	}
	if strings.TrimSpace(h.config.RainbowKey) == "" {
		return gerror.New("请先配置彩虹易支付MD5密钥")
	}
	return nil
}

func (h *rainbowPay) gateway() string {
	gateway := strings.TrimRight(strings.TrimSpace(h.config.RainbowGateway), "/")
	if gateway == "" {
		return defaultGateway
	}
	return gateway
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
