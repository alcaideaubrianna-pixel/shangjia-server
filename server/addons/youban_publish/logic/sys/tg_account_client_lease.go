package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/telegram"

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
