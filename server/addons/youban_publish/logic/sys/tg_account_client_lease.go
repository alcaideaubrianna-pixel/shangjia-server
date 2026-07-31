package sys

import (
	"context"
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

func acquireTelegramAccountClientLease(ctx context.Context, tgAccountId int64) (*lock.Lock, error) {
	if tgAccountId <= 0 {
		return nil, gerror.New("TG账号无效")
	}
	lease := lock.NewConfig(2*time.Minute, time.Second).Mutex(telegramAccountClientLeaseKey(tgAccountId))
	if err := lease.TryLock(ctx); err != nil {
		return nil, gerror.Wrapf(err, "TG账号连接正在使用，拒绝创建第二个客户端 tgAccountId:%d", tgAccountId)
	}
	return lease, nil
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
	lease, err := acquireTelegramAccountClientLease(ctx, tgAccountId)
	if err != nil {
		g.Log().Warningf(ctx, "TG账号连接租约获取失败 tgAccountId:%d err:%+v", tgAccountId, err)
		return err
	}
	defer func() {
		if unlockErr := lease.Unlock(context.Background()); unlockErr != nil {
			g.Log().Warningf(context.Background(), "TG账号连接租约释放失败 tgAccountId:%d err:%+v", tgAccountId, unlockErr)
		}
	}()
	return client.Run(ctx, run)
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
