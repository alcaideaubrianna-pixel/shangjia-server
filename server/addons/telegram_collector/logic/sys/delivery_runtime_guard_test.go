package sys

import (
	"testing"

	"hotgo/addons/telegram_collector/consts"
)

// The collector runtime must consume only the current isolated delivery lanes.
// Legacy queue names remain defined for migration/inspection, but they must not
// receive a live worker and therefore cannot compete with realtime traffic.
func TestDeliveryRuntimeExcludesLegacyQueues(t *testing.T) {
	for name, queues := range map[string]map[string]int{
		"realtime": deliveryRealtimeQueues(),
		"history":  deliveryHistoryQueues(),
	} {
		if _, ok := queues[consts.QueueDeliveryReady]; ok {
			t.Fatalf("%s runtime must not consume legacy ready queue", name)
		}
		if _, ok := queues[consts.QueueDeliveryUrgent]; ok {
			t.Fatalf("%s runtime must not consume legacy urgent queue", name)
		}
	}
}
