package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type unavailableAccountLease struct {
	collectorservice.AccountLease
	err error
}

func (l unavailableAccountLease) Renew(context.Context, *sysin.AccountLease, time.Duration) (bool, error) {
	return false, l.err
}

func TestAccountTaskLeaseFailureStopsParentSession(t *testing.T) {
	original := collectorservice.AccountLeaseManager()
	t.Cleanup(func() { collectorservice.RegisterAccountLease(original) })
	for _, tc := range []struct {
		name     string
		err      error
		canceled bool
	}{
		{name: "expired lease"},
		{name: "Redis unavailable", err: errors.New("Redis LOADING")},
		{name: "normal shutdown", canceled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collectorservice.RegisterAccountLease(unavailableAccountLease{err: tc.err})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.canceled {
				cancel()
			}
			resets := 0
			accountTaskWorkerLoop(ctx, nil, &sysin.AccountLease{AccountID: 58}, &sync.Mutex{}, &sync.Mutex{}, func() {
				resets++
				cancel()
			}, 1)
			want := 1
			if tc.canceled {
				want = 0
			}
			if resets != want {
				t.Fatalf("parent session resets=%d, want %d", resets, want)
			}
			if ctx.Err() == nil {
				t.Fatal("task consumer exited but parent Telegram session is still alive")
			}
		})
	}
}
