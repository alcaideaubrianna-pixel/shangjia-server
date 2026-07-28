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

func (s *sSysPublish) withProfileLifecycleLock(ctx context.Context, tenantId int64, profileId int64, fn func() error) error {
	if tenantId <= 0 || profileId <= 0 {
		return fn()
	}
	lock := hglock.NewConfig(profileLifecycleLockTTL, 200*time.Millisecond).
		Mutex(fmt.Sprintf("youban_publish:profile_lifecycle:%d:%d", tenantId, profileId))
	if err := lock.Lock(ctx); err != nil {
		return gerror.Wrap(err, "获取资料发布状态锁失败")
	}
	defer func() {
		if err := lock.Unlock(ctx); err != nil && !gerror.Is(err, hglock.ErrNotExist) {
			g.Log().Warningf(ctx, "释放资料发布状态锁失败 tenant:%d profile:%d err:%+v", tenantId, profileId, err)
		}
	}()
	return fn()
}
