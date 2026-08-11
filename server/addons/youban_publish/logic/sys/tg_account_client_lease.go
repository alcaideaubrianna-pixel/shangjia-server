package sys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/internal/library/hgrds/lock"
)

const telegramAccountClientLeaseKeyPrefix = "youban_publish:tg:account-client:"

func telegramAccountClientLeaseKey(tgAccountId int64) string {
	return fmt.Sprintf("%s%d", telegramAccountClientLeaseKeyPrefix, tgAccountId)
}

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

func waitTelegramAccountClientLease(
	ctx context.Context,
	retryInterval time.Duration,
	acquire func() (*lock.Lock, error),
	onWait func(error),
) (*lock.Lock, error) {
	if acquire == nil {
		return nil, gerror.New("TG账号连接租约获取函数不能为空")
	}
	if retryInterval <= 0 {
		retryInterval = time.Second
	}
	waitingLogged := false
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lease, err := acquire()
		if err == nil {
			return lease, nil
		}
		if !gerror.Is(err, lock.ErrLockFailed) {
			return nil, err
		}
		observeTelegramLease(ctx, "lock_failed", 0)
		if !waitingLogged && onWait != nil {
			onWait(err)
			waitingLogged = true
		}
		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func acquireTelegramAccountClientLeaseWait(ctx context.Context, tgAccountId int64, onWait func(error)) (*lock.Lock, error) {
	if tgAccountId <= 0 {
		return nil, gerror.New("TG账号无效")
	}
	return waitTelegramAccountClientLease(ctx, time.Second, func() (*lock.Lock, error) {
		lease := lock.NewConfig(2*time.Minute, time.Second).Mutex(telegramAccountClientLeaseKey(tgAccountId))
		if err := lease.TryLock(ctx); err != nil {
			return nil, err
		}
		return lease, nil
	}, onWait)
}

func (s *sSysPublish) runTelegramClientWithAccountLease(ctx context.Context, tgAccountId int64, client *telegram.Client, run func(context.Context) error) error {
	if client == nil {
		return gerror.New("Telegram客户端未初始化")
	}
	if run == nil {
		return gerror.New("Telegram客户端运行函数不能为空")
	}
	leaseCtx, cancel := context.WithTimeout(ctx, telegramAccountClientLeaseWaitTimeout(ctx))
	defer cancel()
	lease, err := acquireTelegramAccountClientLeaseWait(leaseCtx, tgAccountId, func(waitErr error) {
		observeTelegramLease(ctx, "conflict", tgAccountId)
		g.Log().Infof(ctx, "TG账号连接繁忙，等待已有操作完成 tgAccountId:%d err:%+v", tgAccountId, waitErr)
	})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, lock.ErrTimeout) {
			err = &telegramAccountBusyError{tgAccountId: tgAccountId, err: err}
		}
		g.Log().Warningf(ctx, "TG账号连接租约获取失败 tgAccountId:%d err:%+v", tgAccountId, err)
		observeTelegramLease(ctx, "acquire_failed", tgAccountId)
		return err
	}
	observeTelegramLease(ctx, "acquired", tgAccountId)
	observeTelegramLeaseActive(ctx, 1)
	defer func() {
		observeTelegramLeaseActive(context.Background(), -1)
		if unlockErr := lease.Unlock(context.Background()); unlockErr != nil {
			observeTelegramLease(context.Background(), "release_failed", tgAccountId)
			g.Log().Warningf(context.Background(), "TG账号连接租约释放失败 tgAccountId:%d err:%+v", tgAccountId, unlockErr)
		}
		observeTelegramLease(context.Background(), "released", tgAccountId)
	}()
	return client.Run(ctx, run)
}

func telegramAccountClientLeaseWaitTimeout(ctx context.Context) time.Duration {
	seconds := g.Cfg().MustGet(ctx, "youbanPublish.queue.accountBusyTimeoutSeconds", 10).Int()
	if seconds < 1 {
		seconds = 1
	}
	if seconds > 60 {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func (s *sSysPublish) executeTelegramAccountOperation(ctx context.Context, tgAccountId int64, timeout time.Duration, run accountCollectOperation) error {
	return s.executeTelegramAccountOperationMode(ctx, tgAccountId, timeout, run, false)
}

func (s *sSysPublish) executeTelegramAccountMediaOperation(ctx context.Context, tgAccountId int64, timeout time.Duration, run accountCollectOperation) error {
	return s.executeTelegramAccountOperationMode(ctx, tgAccountId, timeout, run, true)
}

func (s *sSysPublish) executeTelegramAccountOperationMode(ctx context.Context, tgAccountId int64, timeout time.Duration, run accountCollectOperation, parallel bool) error {
	if tgAccountId <= 0 || run == nil {
		return gerror.New("TG账号操作参数无效")
	}
	var usedRuntime bool
	var err error
	if parallel {
		usedRuntime, err = s.executeAccountCollectMediaOperation(ctx, tgAccountId, timeout, run)
	} else {
		usedRuntime, err = s.executeAccountCollectOperation(ctx, tgAccountId, timeout, run)
	}
	if err != nil || usedRuntime {
		return err
	}
	return s.executeTelegramAccountStandaloneOperation(ctx, tgAccountId, timeout, run)
}

func (s *sSysPublish) executeTelegramAccountStandaloneOperation(ctx context.Context, tgAccountId int64, timeout time.Duration, run accountCollectOperation) error {
	if tgAccountId <= 0 || run == nil {
		return gerror.New("TG账号操作参数无效")
	}
	account, err := s.accountCollectTgAccount(ctx, tgAccountId)
	if err != nil {
		return err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return err
	}
	client, err := s.newAccountCollectClient(ctx, conf, account, tg.NewUpdateDispatcher())
	if err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return s.runTelegramClientWithAccountLease(runCtx, tgAccountId, client, func(runCtx context.Context) error {
		return run(runCtx, client)
	})
}
