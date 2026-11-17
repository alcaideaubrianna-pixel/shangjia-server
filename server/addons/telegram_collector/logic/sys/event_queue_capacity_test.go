package sys

import (
	"context"
	"testing"

	"hotgo/addons/telegram_collector/consts"
)

func TestCollectorRealtimeCapacityBudget(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		total, requested, bot, account int
	}{
		{"default", 12, 0, 8, 4},
		{"minimum split", 2, 0, 1, 1},
		{"configured", 12, 3, 3, 9},
		{"reserve account", 12, 99, 11, 1},
		{"negative default", 12, -1, 8, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configs := collectorRealtimeServerConfigs(tc.total, tc.requested)
			if len(configs) != 2 || configs[0].Concurrency != tc.account || configs[1].Concurrency != tc.bot {
				t.Fatalf("unexpected split: %+v", configs)
			}
			if configs[0].Concurrency+configs[1].Concurrency != tc.total {
				t.Fatal("total realtime budget increased")
			}
			if len(configs[0].Queues) != 1 || configs[0].Queues[consts.QueueCollectRealtime] != 1 || len(configs[1].Queues) != 1 || configs[1].Queues[consts.QueueCollectFresh] != 1 {
				t.Fatalf("realtime pools overlap: %+v", configs)
			}
		})
	}
}

func TestCollectorSingleRealtimeSlotRemainsShared(t *testing.T) {
	for _, total := range []int{-1, 0, 1} {
		configs := collectorRealtimeServerConfigs(total, 8)
		if len(configs) != 1 || configs[0].Concurrency != 1 || len(configs[0].Queues) != 2 || configs[0].Queues[consts.QueueCollectFresh] != 8 || configs[0].Queues[consts.QueueCollectRealtime] != 4 {
			t.Fatalf("single slot must not create extra capacity: %+v", configs)
		}
	}
}

func TestCollectorBotRealtimeBudgetENV(t *testing.T) {
	t.Setenv("YOUBAN_COLLECTOR_BOT_REALTIME_CONCURRENCY", "3")
	if got := collectorBotRealtimeBudget(context.Background()); got != 3 {
		t.Fatalf("ENV budget=%d want=3", got)
	}
}
