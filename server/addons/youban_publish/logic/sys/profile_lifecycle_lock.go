package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	hglock "hotgo/internal/library/hgrds/lock"
)

const profileLifecycleLockTTL = 2 * time.Minute
const profileLifecycleLockWait = 10 * time.Second

func (s *sSysPublish) withProfileLifecycleLock(ctx context.Context, tenantId int64, profileId int64, fn func() error) error {
	if tenantId <= 0 || profileId <= 0 {
		return fn()
	}
	lock := hglock.NewConfig(profileLifecycleLockTTL, 200*time.Millisecond).
		Mutex(fmt.Sprintf("youban_publish:profile_lifecycle:%d:%d", tenantId, profileId))
	// The Telegram request context may already be cancelled when a concrete API
	// response is being committed. Lock acquisition is a local consistency
	// operation and must have its own bounded budget, otherwise a send timeout
	// turns a successful remote delivery into an unknown job.
	lockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), profileLifecycleLockWait)
	defer cancel()
	started := time.Now()
	var lockErr error
	for attempt := 1; attempt <= 2; attempt++ {
		lockErr = lock.Lock(lockCtx)
		if lockErr == nil {
			break
		}
		if lockCtx.Err() != nil {
			break
		}
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}
	if lockErr != nil {
		g.Log().Warningf(ctx, "获取资料发布状态锁失败 tenant:%d profile:%d elapsed:%s err:%+v", tenantId, profileId, time.Since(started).Round(time.Millisecond), lockErr)
		return gerror.Wrap(lockErr, "获取资料发布状态锁失败")
	}
	defer func() {
		if err := lock.Unlock(ctx); err != nil && !gerror.Is(err, hglock.ErrNotExist) {
			g.Log().Warningf(ctx, "释放资料发布状态锁失败 tenant:%d profile:%d err:%+v", tenantId, profileId, err)
		}
	}()
	return fn()
}
