package runtime

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/util/guid"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/internal/library/hgrds/lock"
)

type accountLeaseHolder struct {
	lease *sysin.AccountLease
	lock  *lock.Lock
}

type sAccountLease struct {
	mu      sync.Mutex
	holders map[int64]*accountLeaseHolder
}

func init() {
	collectorservice.RegisterAccountLease(NewAccountLease())
}

func NewAccountLease() *sAccountLease {
	return &sAccountLease{holders: make(map[int64]*accountLeaseHolder)}
}

func (s *sAccountLease) Acquire(ctx context.Context, accountID int64, instanceID string, ttl time.Duration) (*sysin.AccountLease, bool, error) {
	if accountID <= 0 || strings.TrimSpace(instanceID) == "" {
		return nil, false, gerror.New("Telegram账号租约参数无效")
	}
	if ttl <= 0 {
		ttl = 45 * time.Second
	}
	locker := lock.NewConfig(ttl, time.Second).Mutex(consts.AccountOwnerKey(accountID))
	if err := locker.TryLock(ctx); err != nil {
		if gerror.Is(err, lock.ErrLockFailed) {
			return nil, false, nil
		}
		return nil, false, gerror.Wrap(err, "获取Telegram账号运行租约失败")
	}
	epoch := time.Now().UnixNano()
	if epoch <= 0 {
		epoch = int64(len(guid.S()))
	}
	lease := &sysin.AccountLease{AccountID: accountID, InstanceID: instanceID, Epoch: epoch, ExpiresAt: time.Now().Add(ttl)}
	s.mu.Lock()
	s.holders[epoch] = &accountLeaseHolder{lease: lease, lock: locker}
	s.mu.Unlock()
	return lease, true, nil
}

func (s *sAccountLease) Renew(ctx context.Context, lease *sysin.AccountLease, ttl time.Duration) (bool, error) {
	if lease == nil || lease.AccountID <= 0 || strings.TrimSpace(lease.InstanceID) == "" || lease.Epoch <= 0 {
		return false, gerror.New("Telegram账号租约无效")
	}
	s.mu.Lock()
	holder := s.holders[lease.Epoch]
	if holder == nil || holder.lease.AccountID != lease.AccountID || holder.lease.InstanceID != lease.InstanceID {
		s.mu.Unlock()
		return false, nil
	}
	if ttl <= 0 {
		ttl = 45 * time.Second
	}
	lease.ExpiresAt = time.Now().Add(ttl)
	holder.lease.ExpiresAt = lease.ExpiresAt
	s.mu.Unlock()
	return true, nil
}

func (s *sAccountLease) Release(ctx context.Context, lease *sysin.AccountLease) error {
	if lease == nil {
		return nil
	}
	s.mu.Lock()
	holder := s.holders[lease.Epoch]
	if holder == nil || holder.lease.AccountID != lease.AccountID || holder.lease.InstanceID != lease.InstanceID {
		s.mu.Unlock()
		return nil
	}
	delete(s.holders, lease.Epoch)
	s.mu.Unlock()
	if err := holder.lock.Unlock(ctx); err != nil && !gerror.Is(err, lock.ErrNotExist) {
		return gerror.Wrap(err, "释放Telegram账号运行租约失败")
	}
	return nil
}
