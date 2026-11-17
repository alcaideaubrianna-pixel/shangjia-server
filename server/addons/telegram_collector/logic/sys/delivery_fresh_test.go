package sys

import (
	"context"
	"testing"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestDeliveryFreshCapacityBudget(t *testing.T) {
	for _, total := range []int{1, 2, 3, 4, 8, 12, 32} {
		configs := deliveryRealtimeServerConfigs(total)
		sum := 0
		for _, config := range configs {
			if config.Concurrency < 1 {
				t.Fatalf("budget %d has empty Worker", total)
			}
			sum += config.Concurrency
		}
		if sum != total {
			t.Fatalf("budget %d expanded to %d", total, sum)
		}
		if total == 1 {
			if len(configs) != 1 || configs[0].Queues[consts.QueueDeliveryFresh] <= 0 {
				t.Fatal("single slot must share fresh and backlog")
			}
		} else {
			if len(configs) != 2 || configs[0].Queues[consts.QueueDeliveryFresh] != 0 ||
				len(configs[1].Queues) != 1 || configs[1].Queues[consts.QueueDeliveryFresh] <= 0 {
				t.Fatalf("budget %d does not reserve fresh capacity", total)
			}
		}
		for queue := range deliveryRealtimeQueues() {
			if configs[0].Queues[queue] <= 0 {
				t.Fatalf("budget %d stopped consuming legacy queue %s", total, queue)
			}
		}
	}
	configs := deliveryRealtimeServerConfigs(8)
	if configs[0].Concurrency != 2 || configs[1].Concurrency != 6 {
		t.Fatal("default realtime budget must split backlog=2 fresh=6")
	}
}

func TestDeliveryFreshFirstHandoffOnly(t *testing.T) {
	for _, sourceType := range []string{sysin.SourceTypeBot, sysin.SourceTypeAccount} {
		delivery := &sysin.CollectorDelivery{SourceType: sourceType}
		for _, queue := range []string{consts.QueueCollectFresh, consts.QueueCollectRealtime} {
			if !firstFreshDeliveryHandoff(1, queue, delivery) {
				t.Fatalf("first realtime %s handoff from %s rejected", sourceType, queue)
			}
		}
		for _, attempt := range []int{0, 2, 5} {
			if firstFreshDeliveryHandoff(attempt, consts.QueueCollectRealtime, delivery) {
				t.Fatalf("attempt %d upgraded to fresh", attempt)
			}
		}
		for _, queue := range []string{"", "unknown", consts.QueueCollectHistory, consts.QueueCollectUrgent, consts.QueueCollectUrgentLive} {
			if firstFreshDeliveryHandoff(1, queue, delivery) {
				t.Fatalf("recovery/replay queue %s upgraded to fresh", queue)
			}
		}
		delivery.History = true
		if firstFreshDeliveryHandoff(1, consts.QueueCollectRealtime, delivery) {
			t.Fatal("explicit history upgraded to fresh")
		}
	}
	if firstFreshDeliveryHandoff(1, consts.QueueCollectRealtime, nil) {
		t.Fatal("nil delivery accepted")
	}
	if deliveryTaskNeedsReclassification(consts.QueueDeliveryFresh, 0) {
		t.Fatal("fresh first attempt was demoted")
	}
	if !deliveryTaskNeedsReclassification(consts.QueueDeliveryFresh, 1) {
		t.Fatal("fresh retry would keep reserved capacity")
	}
	if deliveryTaskNeedsReclassification(consts.QueueDeliveryRealtime, 1) {
		t.Fatal("legacy retry changed routing")
	}
}

func TestDeliveryFreshAccountPermits(t *testing.T) {
	var pool deliveryAccountSlotPool
	ctx := context.Background()
	for _, lane := range []deliveryAccountLane{deliveryAccountHistory, deliveryAccountRealtime} {
		release, ok := pool.acquire(ctx, lane, 70, 1)
		if !ok {
			t.Fatal("could not fill backlog account lane")
		}
		defer release()
	}
	lane, limited := deliveryAccountLaneForQueue(consts.QueueDeliveryFresh)
	if !limited || lane != deliveryAccountFresh {
		t.Fatal("fresh queue must use separate account permits")
	}
	release, ok := pool.acquire(ctx, lane, 70, 1)
	if !ok {
		t.Fatal("backlog consumed fresh account permit")
	}
	defer release()
	if extra, ok := pool.acquire(ctx, lane, 70, 1); ok {
		extra()
		t.Fatal("fresh exceeded its account limit")
	}
}
