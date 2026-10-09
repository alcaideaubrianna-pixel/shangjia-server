package sys

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

const collectMediaAccountLeaseKeyPrefix = "youban_publish:collect:media-account-slots"

const collectMediaAccountLeaseAcquireScript = `
redis.call('zremrangebyscore', KEYS[1], '-inf', ARGV[1])
if redis.call('zcard', KEYS[1]) >= tonumber(ARGV[3]) then
    return 0
end
redis.call('zadd', KEYS[1], ARGV[2], ARGV[4])
redis.call('pexpire', KEYS[1], ARGV[5])
return 1
`

const collectMediaAccountLeaseRenewScript = `
if redis.call('zscore', KEYS[1], ARGV[1]) then
    redis.call('zadd', KEYS[1], ARGV[2], ARGV[1])
    redis.call('pexpire', KEYS[1], ARGV[3])
    return 1
end
return 0
`

const collectMediaAccountLeaseReleaseScript = `
local removed = redis.call('zrem', KEYS[1], ARGV[1])
if redis.call('zcard', KEYS[1]) == 0 then
    redis.call('del', KEYS[1])
end
return removed
`

type collectMediaAccountLease struct {
	key         string
	token       string
	ttl         time.Duration
	maxLifetime time.Duration
	stop        chan struct{}
	done        chan struct{}
	once        sync.Once
}

func acquireCollectMediaAccountLease(ctx context.Context, tenantId int64, tgAccountId int64, limit int) (*collectMediaAccountLease, bool, error) {
	if limit < 1 {
		limit = 1
	}
	ttl := time.Duration(g.Cfg().MustGet(ctx, "youbanPublish.collect.accountMediaLeaseSeconds", 120).Int()) * time.Second
	if ttl < 30*time.Second {
		ttl = 30 * time.Second
	}
	if ttl > 10*time.Minute {
		ttl = 10 * time.Minute
	}
	maxLifetime := time.Duration(g.Cfg().MustGet(ctx, "youbanPublish.collect.accountMediaLeaseMaxLifetimeSeconds", 600).Int()) * time.Second
	if maxLifetime < ttl {
		maxLifetime = ttl
	}
	if maxLifetime > 30*time.Minute {
		maxLifetime = 30 * time.Minute
	}
	now := time.Now()
	lease := &collectMediaAccountLease{
		key:         fmt.Sprintf("%s:%d:%d", collectMediaAccountLeaseKeyPrefix, tenantId, tgAccountId),
		token:       guid.S(),
		ttl:         ttl,
		maxLifetime: maxLifetime,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}
	result, err := g.Redis().GroupScript().Eval(ctx, collectMediaAccountLeaseAcquireScript, 1, []string{lease.key}, []interface{}{
		now.UnixMilli(),
		now.Add(ttl).UnixMilli(),
		limit,
		lease.token,
		(ttl * 2).Milliseconds(),
	})
	if err != nil {
		return nil, false, err
	}
	if result.Int() != 1 {
		return nil, false, nil
	}
	go lease.renew(now)
	return lease, true, nil
}

func (lease *collectMediaAccountLease) Release() {
	if lease == nil {
		return
	}
	lease.once.Do(func() {
		close(lease.stop)
		<-lease.done
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = g.Redis().GroupScript().Eval(ctx, collectMediaAccountLeaseReleaseScript, 1, []string{lease.key}, []interface{}{lease.token})
	})
}

func (lease *collectMediaAccountLease) renew(acquiredAt time.Time) {
	defer close(lease.done)
	ticker := time.NewTicker(lease.ttl / 3)
	defer ticker.Stop()
	for {
		select {
		case <-lease.stop:
			return
		case <-ticker.C:
			if time.Since(acquiredAt) >= lease.maxLifetime {
				g.Log().Warningf(context.Background(), "采集媒体账号租约达到最大生命周期，强制释放 key:%s lifetime:%s", lease.key, time.Since(acquiredAt))
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_, _ = g.Redis().GroupScript().Eval(ctx, collectMediaAccountLeaseReleaseScript, 1, []string{lease.key}, []interface{}{lease.token})
				cancel()
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			now := time.Now()
			result, err := g.Redis().GroupScript().Eval(ctx, collectMediaAccountLeaseRenewScript, 1, []string{lease.key}, []interface{}{
				lease.token,
				now.Add(lease.ttl).UnixMilli(),
				(lease.ttl * 2).Milliseconds(),
			})
			if err != nil {
				g.Log().Warningf(ctx, "采集媒体账号租约续期失败 key:%s err:%+v", lease.key, err)
				continue
			}
			if result.Int() != 1 {
				g.Log().Warningf(ctx, "采集媒体账号租约已丢失 key:%s", lease.key)
			}
			cancel()
		}
	}
}
