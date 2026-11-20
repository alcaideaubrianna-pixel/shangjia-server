package sys

import (
	"context"
	"os"
	"strings"

	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/utility/sourcequeue"
)

// Enable only after every collector consumer understands the fresh lane.
// Disabling production leaves the fresh consumer running to drain accepted work.
func deliveryFreshEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED")), "true") || strings.TrimSpace(os.Getenv("YOUBAN_TELEGRAM_DELIVERY_FRESH_ENABLED")) == "1"
}

func firstFreshSourceHandoff(ctx context.Context, attempt int, queue string, delivery *sysin.CollectorDelivery) bool {
	if recovery, _ := ctx.Value(deliverySourceRecoveryKey{}).(bool); recovery {
		return false
	}
	if !deliveryFreshEnabled() || attempt != 1 || delivery == nil || delivery.History || deliveryPriority(delivery) < sysin.EventPriorityRealtime {
		return false
	}
	if lane, ok := collectorSourceLaneFromContext(ctx); ok {
		return lane == sourcequeue.Realtime
	}
	return firstFreshDeliveryHandoff(attempt, queue, delivery)
}

func deliverySourceEnqueueLane(ctx context.Context, priority int) sourcequeue.Lane {
	lane := deliverySourceLane(ctx, priority)
	if lane == sourcequeue.Fresh && (!deliveryFreshEnabled() || priority < sysin.EventPriorityRealtime) {
		return sourcequeue.Realtime
	}
	return lane
}

// Reserve fresh work from the existing budget, never add database concurrency.
// One worker alternates both lanes when the entire budget is one.
func deliverySourceRealtimeBudgets(total int) (fresh, backlog int) {
	if total < 1 {
		total = 1
	}
	if total == 1 {
		return 1, 0
	}
	backlog = max(1, total/4)
	return total - backlog, backlog
}
