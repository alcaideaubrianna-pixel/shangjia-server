package sys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/library/hgrds/lock"
)

const (
	collectMediaDefaultSourceConcurrency  = 2
	collectMediaDefaultAccountConcurrency = 4
	collectMediaMaxOwnerConcurrency       = 16
)

type collectMediaConcurrencyLease struct {
	locks []*lock.Lock
}

func (l *collectMediaConcurrencyLease) refresh(ctx context.Context) (bool, error) {
	for _, lease := range l.locks {
		owned, err := lease.Refresh(ctx)
		if err != nil || !owned {
			return false, err
		}
	}
	return true, nil
}

func (l *collectMediaConcurrencyLease) release() {
	if l == nil {
		return
	}
	for index := len(l.locks) - 1; index >= 0; index-- {
		_ = l.locks[index].Unlock(context.Background())
	}
}

func (s *sSysPublish) acquireCollectMediaOwnerSlots(ctx context.Context, payload collectMediaQueuePayload) (*collectMediaConcurrencyLease, error) {
	lease := new(collectMediaConcurrencyLease)
	accountLimit := normalizeCollectMediaOwnerConcurrency(g.Cfg().MustGet(ctx, "youbanPublish.collect.mediaAccountConcurrency", collectMediaDefaultAccountConcurrency).Int())
	sourceLimit := normalizeCollectMediaOwnerConcurrency(g.Cfg().MustGet(ctx, "youbanPublish.collect.mediaSourceConcurrency", collectMediaDefaultSourceConcurrency).Int())
	if payload.TgAccountId > 0 {
		accountLock, err := acquireCollectMediaOwnerSlot(ctx, "account", payload.TenantId, payload.TgAccountId, payload.EventId, accountLimit)
		if err != nil {
			observeCollectMediaScheduler(ctx, "account_concurrency_full", payload.TenantId, payload.TgAccountId)
			return nil, err
		}
		lease.locks = append(lease.locks, accountLock)
	}
	sourceLock, err := acquireCollectMediaOwnerSlot(ctx, "source", payload.TenantId, payload.SourceId, payload.EventId, sourceLimit)
	if err != nil {
		lease.release()
		observeCollectMediaScheduler(ctx, "source_concurrency_full", payload.TenantId, payload.TgAccountId)
		return nil, err
	}
	lease.locks = append(lease.locks, sourceLock)
	return lease, nil
}

func acquireCollectMediaOwnerSlot(ctx context.Context, kind string, tenantID, ownerID, eventID int64, limit int) (*lock.Lock, error) {
	if tenantID <= 0 || ownerID <= 0 || eventID <= 0 {
		return nil, fmt.Errorf("采集媒体并发参数无效 kind:%s tenantId:%d ownerId:%d eventId:%d", kind, tenantID, ownerID, eventID)
	}
	start := int(eventID % int64(limit))
	for offset := 0; offset < limit; offset++ {
		slot := (start + offset) % limit
		locker := lock.NewConfig(collectMediaLeaseTTL, time.Second).WithoutWatchDog().Mutex(fmt.Sprintf("youban_publish:collect:media:%s:%d:%d:slot:%d", kind, tenantID, ownerID, slot))
		err := locker.TryLock(ctx)
		if err == nil {
			return locker, nil
		}
		if !errors.Is(err, lock.ErrLockFailed) {
			return nil, err
		}
	}
	return nil, newCollectMediaFairnessRetryError("采集媒体并发已满，等待下一轮公平调度", 3*time.Second)
}

func normalizeCollectMediaOwnerConcurrency(limit int) int {
	if limit < 1 {
		return 1
	}
	if limit > collectMediaMaxOwnerConcurrency {
		return collectMediaMaxOwnerConcurrency
	}
	return limit
}
