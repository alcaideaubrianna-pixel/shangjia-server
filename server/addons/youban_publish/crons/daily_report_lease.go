package crons

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/hgrds/lock"
)

// A report poll has at most 50 seconds of work. The fixed lease prevents
// overlapping scheduler instances from repeating the same expensive scan.
func withDailyReportLease(ctx context.Context, run func() error) error {
	lease := lock.NewConfig(55*time.Second, time.Second).WithoutWatchDog().Mutex(consts.DailyReportCoordinatorLeaseKey)
	claimCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := lease.TryLock(claimCtx)
	cancel()
	if gerror.Is(err, lock.ErrLockFailed) {
		return nil
	}
	if err != nil {
		return gerror.Wrap(err, "获取日报调度租约失败")
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if err := lease.Unlock(releaseCtx); err != nil {
			g.Log().Warningf(releaseCtx, "释放日报调度租约失败 err:%+v", err)
		}
	}()
	return run()
}
