// Package rainbow 彩虹易支付
package rainbow

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"hotgo/internal/consts"
	"hotgo/internal/library/location"
	"hotgo/internal/model"
	"hotgo/internal/model/input/payin"
	"hotgo/utility/validate"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
)

const (
	defaultGateway = "https://pay.v8jisu.cn"
	defaultMethod  = "jump"
	signTypeRSA    = "RSA"
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
	if err = h.validateConfig(false); err != nil {
		return
	}

	request := ghttp.RequestFromCtx(ctx)
	params := request.GetQueryMap()
	signParamsMap := make(map[string]string, len(params))
	for key, value := range params {
		signParamsMap[key] = gconv.String(value)
	}

	if err = verifyParams(signParamsMap, h.config.RainbowPlatformPublicKey); err != nil {
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
	if err = h.validateConfig(true); err != nil {
		return
	}
	if in.Pay == nil {
		err = gerror.New("支付订单不能为空")
		return
	}

	tradeType := strings.TrimSpace(in.Pay.TradeType)
	if tradeType == "" {
		tradeType = consts.TradeTypeRainbowAliPay
	}

	request := ghttp.RequestFromCtx(ctx)
	device := "pc"
	if request != nil && validate.IsMobileVisit(request.UserAgent()) {
		device = "mobile"
	}

	params := map[string]string{
		"pid":          strings.TrimSpace(h.config.RainbowPid),
		"method":       h.method(),
		"device":       device,
		"type":         tradeType,
		"out_trade_no": in.Pay.OutTradeNo,
		"notify_url":   in.Pay.NotifyUrl,
		"return_url":   in.Pay.ReturnUrl,
		"name":         in.Pay.Subject,
		"money":        fmt.Sprintf("%.2f", in.Pay.PayAmount),
		"clientip":     in.Pay.CreateIp,
		"timestamp":    fmt.Sprintf("%d", time.Now().Unix()),
		"sign_type":    signTypeRSA,
	}
	if params["clientip"] == "" && request != nil {
		params["clientip"] = location.GetClientIp(request)
	}

	params["sign"], err = signParams(params, h.config.RainbowPrivateKey)
	if err != nil {
		return
	}

	response, err := h.postForm(ctx, params)
	if err != nil {
		return
	}
	if response == nil || response.Code != 0 {
		msg := ""
		if response != nil {
			msg = response.Msg
		}
		if msg == "" {
			msg = "彩虹易支付下单失败"
		}
		err = gerror.New(msg)
		return
	}

	res = &payin.CreateOrderModel{
		TradeType:  tradeType,
		PayURL:     response.PayInfo,
		OutTradeNo: in.Pay.OutTradeNo,
	}
	return
}

func (h *rainbowPay) postForm(ctx context.Context, params map[string]string) (*createResponse, error) {
	form := url.Values{}
	for key, value := range params {
		if value != "" {
			form.Set(key, value)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.gateway()+"/api/pay/create", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, gerror.Wrap(err, "创建彩虹易支付请求失败")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, gerror.Wrap(err, "请求彩虹易支付失败")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, gerror.Wrap(err, "读取彩虹易支付响应失败")
	}

	var out *createResponse
	if err = json.Unmarshal(body, &out); err != nil {
		return nil, gerror.Wrap(err, "解析彩虹易支付响应失败")
	}
	return out, nil
}

func (h *rainbowPay) validateConfig(checkPrivate bool) error {
	if h.config == nil {
		return gerror.New("支付配置未初始化")
	}
	if strings.TrimSpace(h.config.RainbowPid) == "" {
		return gerror.New("请先配置彩虹易支付商户ID")
	}
	if checkPrivate && strings.TrimSpace(h.config.RainbowPrivateKey) == "" {
		return gerror.New("请先配置彩虹易支付商户私钥")
	}
	if !checkPrivate && strings.TrimSpace(h.config.RainbowPlatformPublicKey) == "" {
		return gerror.New("请先配置彩虹易支付平台公钥")
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

func (h *rainbowPay) method() string {
	method := strings.TrimSpace(h.config.RainbowMethod)
	if method == "" {
		return defaultMethod
	}
	return method
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
