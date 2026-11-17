package sys

import (
	"context"
	"testing"

	"hotgo/addons/telegram_collector/consts"
)

func TestDeliveryAccountSlotsIsolateRealtimeAndHistory(t *testing.T) {
	ctx := context.Background()
	for _, first := range []deliveryAccountLane{deliveryAccountHistory, deliveryAccountRealtime} {
		var pool deliveryAccountSlotPool
		other := deliveryAccountRealtime
		if first == other {
			other = deliveryAccountHistory
		}
		var releases []func()
		for i := 0; i < 2; i++ {
			release, ok := pool.acquire(ctx, first, 70, 2)
			if !ok {
				t.Fatal("could not fill first lane")
			}
			releases = append(releases, release)
		}
		if release, ok := pool.acquire(ctx, first, 70, 2); ok {
			release()
			t.Fatal("same lane exceeded account limit")
		}
		for i := 0; i < 2; i++ {
			release, ok := pool.acquire(ctx, other, 70, 2)
			if !ok {
				t.Fatal("other lane blocked by full first lane")
			}
			releases = append(releases, release)
		}
		if release, ok := pool.acquire(ctx, other, 70, 2); ok {
			release()
			t.Fatal("other lane exceeded its own account limit")
		}
		for _, release := range releases {
			release()
		}
	}
}

func TestDeliveryAccountSlotsAccountIsolationAndRelease(t *testing.T) {
	var pool deliveryAccountSlotPool
	ctx := context.Background()
	release, ok := pool.acquire(ctx, deliveryAccountHistory, 70, 1)
	if !ok {
		t.Fatal("initial acquisition failed")
	}
	otherRelease, ok := pool.acquire(ctx, deliveryAccountHistory, 71, 1)
	if !ok {
		t.Fatal("another account blocked")
	}
	otherRelease()
	release()
	release() // Repeated cleanup must not block or release another permit.
	reacquired, ok := pool.acquire(ctx, deliveryAccountHistory, 70, 1)
	if !ok {
		t.Fatal("released permit unavailable")
	}
	defer reacquired()
	if unexpected, ok := pool.acquire(ctx, deliveryAccountHistory, 70, 2); ok {
		unexpected()
		t.Fatal("changing config replaced an occupied semaphore")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if unexpected, ok := pool.acquire(cancelled, deliveryAccountRealtime, 70, 1); ok {
		unexpected()
		t.Fatal("cancelled acquisition succeeded")
	}
}

func TestDeliveryAccountLaneForQueue(t *testing.T) {
	for _, tt := range []struct {
		queue string
		lane  deliveryAccountLane
	}{
		{consts.QueueDeliveryHistory, deliveryAccountHistory},
		{consts.DeliveryHistoryShardQueue(70), deliveryAccountHistory},
		{consts.DeliveryRealtimeShardQueue(70), deliveryAccountRealtime},
		{consts.DeliveryUrgentShardQueue(70), deliveryAccountRealtime},
	} {
		lane, limited := deliveryAccountLaneForQueue(tt.queue)
		if !limited || lane != tt.lane {
			t.Fatalf("queue %s: lane=%d limited=%t", tt.queue, lane, limited)
		}
	}
	for _, queue := range []string{consts.QueueDeliveryReady, consts.QueueDeliveryRealtime,
		consts.QueueDeliveryUrgentLive, consts.DeliveryLiveSourceQueue(351), consts.DeliveryHistorySourceQueue(351)} {
		if _, limited := deliveryAccountLaneForQueue(queue); limited {
			t.Fatalf("unexpected account routing query for queue %s", queue)
		}
	}
}
