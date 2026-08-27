// Package gmpay GMPay 推荐接入
package gmpay

import (
	"context"
	"net/url"
	"path"
	"strings"
	"time"

	"hotgo/internal/model"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/payin"

	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
)

const (
	defaultCurrency = "CNY"
	gmpayOrderAPI   = "/payments/gmpay/v1/order/create-transaction"
)

func New(config *model.PayConfig) *GMPay {
	return &GMPay{config: config}
}

type GMPay struct {
	config *model.PayConfig
}

func (h *GMPay) Refund(ctx context.Context, in payin.RefundInp) (res *payin.RefundModel, err error) {
	err = gerror.New("GMPay 暂未接入退款，如有疑问请联系管理员")
	return
}

func (h *GMPay) Notify(ctx context.Context, in payin.NotifyInp) (res *payin.NotifyModel, err error) {
	if err = h.validateConfig(); err != nil {
		return
	}

	params := h.requestParams(ghttp.RequestFromCtx(ctx))
	if err = verifyParams(params, h.config.GMPayKey); err != nil {
		return
	}

	var notify *notifyRequest
	if err = gconv.Scan(params, &notify); err != nil {
		return
	}
	if notify == nil {
		err = gerror.New("解析 GMPay 回调参数失败")
		return
	}
	if !isPaymentSuccess(notify) {
		err = gerror.Newf("非交易支付成功状态，无需处理：status=%d status_code=%d code=%d", notify.Status, notify.StatusCode, notify.Code)
		return
	}
	if strings.TrimSpace(notify.OrderID) == "" {
		err = gerror.New("GMPay 回调缺少商户订单号")
		return
	}

	payAt := gtime.Now()
	res = &payin.NotifyModel{
		OutTradeNo:    notify.OrderID,
		TransactionId: firstNonEmpty(notify.BlockTransactionID, notify.TradeID, notify.OrderID),
		PayAt:         payAt,
		ActualAmount:  firstPositive(notify.ActualAmount, notify.Amount),
	}
	return
}

func isPaymentSuccess(notify *notifyRequest) bool {
	if notify == nil {
		return false
	}
	if notify.Status == 2 {
		return true
	}
	if notify.Status == 0 && notify.StatusCode == 200 {
		return true
	}
	return notify.Status == 0 && notify.StatusCode == 0 && notify.Code == 200
}

func (h *GMPay) CreateOrder(ctx context.Context, in payin.CreateOrderInp) (res *payin.CreateOrderModel, err error) {
	if err = h.validateConfig(); err != nil {
		return
	}
	if in.Pay == nil {
		err = gerror.New("支付订单不能为空")
		return
	}

	req, err := h.createRequest(in.Pay)
	if err != nil {
		return nil, err
	}
	req.Signature = signParams(h.requestSignMap(req), h.config.GMPayKey)

	resp, err := g.Client().SetTimeout(15*time.Second).ContentJson().Post(ctx, h.gateway()+gmpayOrderAPI, req)
	if err != nil {
		return
	}
	defer func() {
		_ = resp.Close()
	}()

	rsp, err := h.parseCreateResponse(resp.ReadAllString())
	if err != nil {
		return
	}

	res = &payin.CreateOrderModel{
		TradeType:      in.Pay.TradeType,
		PayURL:         rsp.PaymentURL,
		OutTradeNo:     in.Pay.OutTradeNo,
		TradeID:        firstNonEmpty(rsp.Data.TradeID, rsp.TradeID),
		Currency:       rsp.Data.Currency,
		Token:          rsp.Data.Token,
		Network:        rsp.Data.Network,
		ActualAmount:   firstPositive(rsp.Data.ActualAmount, rsp.Data.Amount),
		ReceiveAddress: rsp.Data.ReceiveAddress,
	}
	return
}

func (h *GMPay) createRequest(pay *entity.PayLog) (createTransactionRequest, error) {
	if pay == nil {
		return createTransactionRequest{}, gerror.New("支付订单不能为空")
	}
	req := createTransactionRequest{
		Pid:         strings.TrimSpace(h.config.GMPayPid),
		OrderID:     pay.OutTradeNo,
		Amount:      pay.PayAmount,
		Currency:    defaultCurrency,
		NotifyURL:   pay.NotifyUrl,
		RedirectURL: pay.ReturnUrl,
		Name:        pay.Subject,
	}
	req.Token = strings.TrimSpace(h.config.GMPayToken)
	req.Network = strings.TrimSpace(h.config.GMPayNetwork)
	if (req.Token == "") != (req.Network == "") {
		return createTransactionRequest{}, gerror.New("GMPay token 和 network 必须同时配置或同时留空")
	}
	return req, nil
}

func (h *GMPay) requestParams(request *ghttp.Request) map[string]string {
	params := make(map[string]string)
	if request == nil {
		return params
	}

	if body, err := request.GetJson(); err == nil && body != nil && len(body.Map()) > 0 {
		for key, value := range body.Map() {
			params[key] = gconv.String(value)
		}
		return params
	}

	values := request.GetMap()
	for key, value := range values {
		params[key] = gconv.String(value)
	}
	return params
}

func (h *GMPay) requestSignMap(req createTransactionRequest) map[string]string {
	return map[string]string{
		"pid":          req.Pid,
		"order_id":     req.OrderID,
		"amount":       gconv.String(req.Amount),
		"currency":     req.Currency,
		"notify_url":   req.NotifyURL,
		"redirect_url": req.RedirectURL,
		"name":         req.Name,
		"token":        req.Token,
		"network":      req.Network,
	}
}

func (h *GMPay) parseCreateResponse(raw string) (rsp *createTransactionResponse, err error) {
	json := gjson.New(raw)
	if json == nil || json.IsNil() {
		err = gerror.Newf("解析 GMPay 返回失败：%s", raw)
		return
	}

	rsp = &createTransactionResponse{
		StatusCode: json.Get("status_code").Int(),
		Code:       json.Get("code").Int(),
		Message:    firstNonEmpty(json.Get("message").String(), json.Get("msg").String()),
		RequestID:  json.Get("request_id").String(),
	}
	if rsp.StatusCode == 0 {
		rsp.StatusCode = rsp.Code
	}
	rsp.Data.PaymentURL = firstNonEmpty(
		json.Get("data.payment_url").String(),
		json.Get("data.paymentUrl").String(),
		json.Get("payment_url").String(),
		json.Get("paymentUrl").String(),
	)
	rsp.Data.TradeID = firstNonEmpty(
		json.Get("data.trade_id").String(),
		json.Get("data.tradeId").String(),
		json.Get("trade_id").String(),
		json.Get("tradeId").String(),
	)
	rsp.Data.OrderID = firstNonEmpty(
		json.Get("data.order_id").String(),
		json.Get("data.orderId").String(),
		json.Get("order_id").String(),
		json.Get("orderId").String(),
	)
	rsp.Data.Currency = firstNonEmpty(
		json.Get("data.currency").String(),
		json.Get("currency").String(),
	)
	rsp.Data.Token = firstNonEmpty(
		json.Get("data.token").String(),
		json.Get("token").String(),
	)
	rsp.Data.Network = firstNonEmpty(
		json.Get("data.network").String(),
		json.Get("network").String(),
	)
	rsp.Data.ReceiveAddress = firstNonEmpty(
		json.Get("data.receive_address").String(),
		json.Get("data.receiveAddress").String(),
		json.Get("receive_address").String(),
		json.Get("receiveAddress").String(),
	)
	rsp.Data.Amount = firstPositive(
		json.Get("data.amount").Float64(),
		json.Get("amount").Float64(),
	)
	rsp.Data.ActualAmount = firstPositive(
		json.Get("data.actual_amount").Float64(),
		json.Get("data.actualAmount").Float64(),
		json.Get("actual_amount").Float64(),
		json.Get("actualAmount").Float64(),
	)
	rsp.PaymentURL = rsp.Data.PaymentURL
	rsp.TradeID = rsp.Data.TradeID
	rsp.OrderID = rsp.Data.OrderID
	rsp.PaymentURL = normalizePaymentURL(rsp.PaymentURL)

	if rsp.StatusCode != 200 {
		message := firstNonEmpty(rsp.Message, raw)
		if rsp.RequestID != "" {
			message += "，request_id=" + rsp.RequestID
		}
		err = gerror.Newf("GMPay 创建订单失败：%s", message)
		return
	}
	if rsp.PaymentURL == "" {
		err = gerror.Newf("GMPay 创建订单失败，返回缺少 payment_url：%s", raw)
		return
	}
	return
}

func (h *GMPay) validateConfig() error {
	if h.config == nil {
		return gerror.New("支付配置未初始化")
	}
	if strings.TrimSpace(h.config.GMPayPid) == "" {
		return gerror.New("请先配置 GMPay 商户 PID")
	}
	if strings.TrimSpace(h.config.GMPayKey) == "" {
		return gerror.New("请先配置 GMPay 密钥")
	}
	return nil
}

func (h *GMPay) gateway() string {
	gateway := strings.TrimRight(strings.TrimSpace(h.config.GMPayGateway), "/")
	if gateway == "" {
		gateway = "http://127.0.0.1:18000"
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

func firstPositive(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func normalizePaymentURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return strings.TrimRight(raw, "/")
	}

	cleanedPath := path.Clean(parsed.Path)
	if cleanedPath == "." {
		cleanedPath = ""
	}
	if cleanedPath == "/" {
		parsed.Path = cleanedPath
	} else {
		parsed.Path = cleanedPath
	}

	return strings.TrimRight(parsed.String(), "/")
}
