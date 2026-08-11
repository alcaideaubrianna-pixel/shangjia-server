package service

import (
	"context"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

type AccountLease interface {
	Acquire(ctx context.Context, accountID int64, instanceID string, ttl time.Duration) (*sysin.AccountLease, bool, error)
	Renew(ctx context.Context, lease *sysin.AccountLease, ttl time.Duration) (bool, error)
	Release(ctx context.Context, lease *sysin.AccountLease) error
}

var localAccountLease AccountLease

func AccountLeaseManager() AccountLease {
	if localAccountLease == nil {
		panic("telegram account lease service is not registered")
	}
	return localAccountLease
}

func RegisterAccountLease(value AccountLease) { localAccountLease = value }
