// Package payment
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Ms <133814250@qq.com>
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package payment

import (
	"context"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/internal/model/input/payin"
	"sync"
)

// 异步回调

type NotifyCallFunc func(ctx context.Context, pay *payin.NotifyCallFuncInp) (err error)

var (
	notifyCall = make(map[string]NotifyCallFunc)
	ncLock     sync.Mutex
)

// RegisterNotifyCall 注册支付成功回调方法
func RegisterNotifyCall(group string, f NotifyCallFunc) {
	ncLock.Lock()
	defer ncLock.Unlock()
	if _, ok := notifyCall[group]; ok {
		panic("notifyCall repeat registration, group:" + group)
	}
	notifyCall[group] = f
}

// RegisterNotifyCallMap 注册支付成功回调方法
func RegisterNotifyCallMap(calls map[string]NotifyCallFunc) {
	for group, f := range calls {
		RegisterNotifyCall(group, f)
	}
}

// NotifyCall 执行订单分组的回调
func NotifyCall(ctx context.Context, in *payin.NotifyCallFuncInp) error {
	f, ok := notifyCall[in.Pay.OrderGroup]
	if ok {
		if err := f(ctx, in); err != nil {
			g.Log().Warningf(ctx, "payment.NotifyCall in:%+v exec err:%+v", gjson.New(in.Pay).String(), err)
			return err
		}
	}
	return nil
}
