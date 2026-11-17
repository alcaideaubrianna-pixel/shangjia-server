package sys

import (
	"context"
	"strings"
	"sync"

	"hotgo/addons/telegram_collector/consts"
)

type deliveryAccountLane uint8

const (
	deliveryAccountHistory deliveryAccountLane = iota + 1
	deliveryAccountRealtime
	deliveryAccountFresh
)

type deliveryAccountSlotKey struct {
	lane      deliveryAccountLane
	accountID int64
}

// These are local account fairness permits, not additional Worker capacity.
// The Asynq servers continue to bound total in-flight work within their budgets.
type deliveryAccountSlotPool struct {
	mu    sync.Mutex
	slots map[deliveryAccountSlotKey]chan struct{}
}

// Preserve the existing account-limited queue set. Source-only queues have
// no account identity and do not need an extra routing query or semaphore.
func deliveryAccountLaneForQueue(queue string) (deliveryAccountLane, bool) {
	switch {
	case queue == consts.QueueDeliveryFresh:
		return deliveryAccountFresh, true
	case queue == consts.QueueDeliveryHistory,
		strings.HasPrefix(queue, consts.QueueDeliveryHistoryShardPrefix):
		return deliveryAccountHistory, true
	case strings.HasPrefix(queue, consts.QueueDeliveryRealtimeShardPrefix),
		strings.HasPrefix(queue, consts.QueueDeliveryUrgentShardPrefix):
		return deliveryAccountRealtime, true
	default:
		return 0, false
	}
}

func (p *deliveryAccountSlotPool) acquire(ctx context.Context, lane deliveryAccountLane, accountID int64, limit int) (func(), bool) {
	noop := func() {}
	if ctx.Err() != nil {
		return noop, false
	}
	if accountID <= 0 {
		return noop, true
	}
	if limit < 1 || (lane != deliveryAccountHistory && lane != deliveryAccountRealtime && lane != deliveryAccountFresh) {
		return noop, false
	}
	key := deliveryAccountSlotKey{lane: lane, accountID: accountID}
	p.mu.Lock()
	if p.slots == nil {
		p.slots = make(map[deliveryAccountSlotKey]chan struct{})
	}
	slots := p.slots[key]
	if slots == nil {
		// Keep the pool stable for this runtime. Replacing a channel while
		// permits are held would temporarily admit more than the limit.
		slots = make(chan struct{}, limit)
		p.slots[key] = slots
	}
	p.mu.Unlock()
	select {
	case slots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-slots }) }, true
	default:
		return noop, false
	}
}
