// Package pay
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package pay

// 订单创建

import (
	"context"
	"fmt"
	v1 "hotgo/api/api/pay/v1"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/location"
	"hotgo/internal/library/payment"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/payin"
	"hotgo/internal/service"
	"hotgo/utility/validate"
	"net/url"
	"strings"

	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gmeta"
)

// Create 创建支付订单和日志
func (s *sPay) Create(ctx context.Context, in payin.PayCreateInp) (res *payin.PayCreateModel, err error) {
	request := ghttp.RequestFromCtx(ctx)
	if in.TradeType == "" {
		in.TradeType = payment.AutoTradeType(in.PayType, request.UserAgent())
	}

	if in.PayType == consts.PayTypeWxPay && in.Openid == "" {
		if in.Openid, err = service.CommonWechat().GetOpenId(ctx); err != nil {
			return
		}
	}

	if in.TradeType == consts.TradeTypeWxMP {
		if in.Openid == "" {
			err = gerror.New("微信公众号支付必须设置openid")
			return
		}
		if in.ReturnUrl == "" {
			err = gerror.New("微信公众号支付必须设置同步通知地址")
			return
		}
	}

	notifyURL, err := s.GenNotifyURL(ctx, in)
	if err != nil {
		return
	}

	config := payment.GetConfig()
	mchId := ""
	switch in.PayType {
	case consts.PayTypeAliPay:
		mchId = config.AliPayAppId
	case consts.PayTypeWxPay:
		mchId = config.WxPayMchId
	case consts.PayTypeQQPay:
		mchId = config.QQPayMchId
	case consts.PayTypeGMPay:
		mchId = config.GMPayPid
	case consts.PayTypeRainbow:
		mchId = config.RainbowPid
	}

	data := &entity.PayLog{
		MemberId:      contexts.GetUserId(ctx),
		AppId:         contexts.GetModule(ctx),
		AddonsName:    contexts.GetAddonName(ctx),
		OrderSn:       in.OrderSn,
		OrderGroup:    in.OrderGroup,
		Openid:        in.Openid,
		MchId:         mchId,
		Subject:       in.Subject,
		Detail:        in.Detail,
		OutTradeNo:    payment.GenOutTradeNo(ctx),
		TransactionId: "",
		PayType:       in.PayType,
		PayAmount:     in.PayAmount,
		PayStatus:     consts.PayStatusWait,
		TradeType:     in.TradeType,
		IsRefund:      consts.RefundStatusNo,
		CreateIp:      location.GetClientIp(request),
		NotifyUrl:     notifyURL,
		ReturnUrl:     in.ReturnUrl,
		TraceIds:      gjson.New([]string{gctx.CtxId(ctx)}),
		Status:        consts.StatusEnabled,
	}

	// 创建支付记录
	if data.PayAmount <= 0 {
		now := gtime.Now()
		data.PayStatus = consts.PayStatusOk
		data.PayAt = now
		data.ActualAmount = 0
		data.TransactionId = data.OutTradeNo
	}

	if _, err = s.Model(ctx).Data(data).OmitEmptyData().Insert(); err != nil {
		return
	}

	if data.PayAmount <= 0 {
		res = &payin.PayCreateModel{
			Order: &payin.CreateOrderModel{
				TradeType:  in.TradeType,
				OutTradeNo: data.OutTradeNo,
			},
		}
		return
	}

	// 创建第三方平台支付订单
	order, err := payment.New(in.PayType).CreateOrder(ctx, payin.CreateOrderInp{Pay: data})
	if err != nil {
		return
	}

	res = new(payin.PayCreateModel)
	res.Order = order
	return
}

// GenNotifyURL 生成支付通知地址
func (s *sPay) GenNotifyURL(ctx context.Context, in payin.PayCreateInp) (notifyURL string, err error) {
	basic, err := service.SysConfig().GetBasic(ctx)
	if err != nil {
		return
	}

	if basic.Domain == "" {
		err = gerror.New("请先到后台【系统设置】-【配置管理】中设置网站域名！")
		return
	}

	if !validate.IsURL(basic.Domain) {
		err = gerror.New("网站域名格式有误，请检查！")
		return
	}

	baseURL, err := validateNotifyBaseURL(basic.Domain)
	if err != nil {
		return
	}

	var object interface{}
	switch in.PayType {
	case consts.PayTypeAliPay:
		object = v1.NotifyAliPayReq{}
	case consts.PayTypeWxPay:
		object = v1.NotifyWxPayReq{}
	case consts.PayTypeQQPay:
		object = v1.NotifyQQPayReq{}
	case consts.PayTypeGMPay:
		object = v1.NotifyGMPayReq{}
	case consts.PayTypeRainbow:
		object = v1.NotifyRainbowReq{}
	default:
		err = gerror.Newf("未被支持的支付方式：%v", in.PayType)
		return
	}

	notifyURL = fmt.Sprintf("%s%s%s",
		baseURL,
		g.Cfg().MustGet(ctx, "router.api.prefix", "/api").String(),
		gmeta.Get(object, "path").String(),
	)
	return
}

func validateNotifyBaseURL(domain string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(domain), "/")
	if base == "" {
		return "", gerror.New("请先到后台【系统设置】-【配置管理】中设置网站域名！")
	}

	parsed, err := url.Parse(base)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", gerror.New("网站域名格式有误，请检查！")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", gerror.New("网站域名必须以 http:// 或 https:// 开头")
	}

	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "localhost", "127.0.0.1", "0.0.0.0":
		return "", gerror.New("GMPay notify_url 必须使用可被网关访问的公网域名，不能使用 localhost 或 127.0.0.1")
	}

	return base, nil
}
