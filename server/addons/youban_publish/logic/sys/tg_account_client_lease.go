package sys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	collectorservice "hotgo/addons/telegram_collector/service"
)

// telegramAccountBusyError remains for retry-policy compatibility with tasks
// persisted by older versions. New account operations use AccountRuntime.
type telegramAccountBusyError struct {
	tgAccountId int64
	err         error
}

func (e *telegramAccountBusyError) Error() string {
	if e == nil || e.err == nil {
		return fmt.Sprintf("TG账号连接正在使用，等待账号连接释放 tgAccountId:%d", e.tgAccountId)
	}
	return fmt.Sprintf("TG账号连接正在使用，等待账号连接释放 tgAccountId:%d: %v", e.tgAccountId, e.err)
}

func (e *telegramAccountBusyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func isTelegramAccountBusyError(err error) bool {
	var busyErr *telegramAccountBusyError
	return errors.As(err, &busyErr)
}

func (s *sSysPublish) executeTelegramAccountOperation(ctx context.Context, tgAccountId int64, timeout time.Duration, run accountCollectOperation) error {
	if tgAccountId <= 0 || run == nil {
		return gerror.New("TG账号操作参数无效")
	}
	return s.executeTelegramAccountRuntimeUntilReady(ctx, tgAccountId, timeout, run, false)
}

func (s *sSysPublish) executeTelegramAccountPriorityOperation(ctx context.Context, tgAccountId int64, timeout time.Duration, run accountCollectOperation) error {
	if tgAccountId <= 0 || run == nil {
		return gerror.New("TG账号操作参数无效")
	}
	return s.executeTelegramAccountRuntimeUntilReady(ctx, tgAccountId, timeout, run, true)
}

func (s *sSysPublish) executeTelegramAccountRuntimeUntilReady(ctx context.Context, tgAccountId int64, timeout time.Duration, run accountCollectOperation, priority bool) error {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	waitingLogged := false
	for {
		var used bool
		var err error
		operation := collectorservice.AccountOperation(run)
		if priority {
			used, err = collectorservice.AccountRuntime().ExecutePriority(runCtx, tgAccountId, timeout, operation)
		} else {
			used, err = collectorservice.AccountRuntime().Execute(runCtx, tgAccountId, timeout, operation)
		}
		if err != nil || used {
			return err
		}
		if !waitingLogged {
			g.Log().Infof(runCtx, "TG账号操作等待常驻客户端 tgAccountId:%d priority:%t", tgAccountId, priority)
			waitingLogged = true
		}
		collectorservice.AccountRuntime().Refresh()
		if err = waitTelegramAccountRuntimeRefresh(runCtx, 250*time.Millisecond); err != nil {
			return gerror.New("TG账号常驻客户端正在启动，请稍后重试")
		}
	}
}

func waitTelegramAccountRuntimeRefresh(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
