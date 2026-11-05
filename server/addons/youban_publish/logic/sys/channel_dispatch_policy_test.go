package sys

import "testing"

// TestChannelDispatchQueueName 验证发送和删除使用完全隔离的队列。
func TestChannelDispatchQueueName(t *testing.T) {
	if got := channelDispatchQueueName(channelDispatchLaneSend); got != channelDispatchQueueSend {
		t.Fatalf("send queue = %q, want %q", got, channelDispatchQueueSend)
	}
	if got := channelDispatchQueueName(channelDispatchLaneDelete); got != channelDispatchQueueDelete {
		t.Fatalf("delete queue = %q, want %q", got, channelDispatchQueueDelete)
	}
	if channelDispatchQueueSend == channelDispatchQueueDelete {
		t.Fatal("send and delete queues must be isolated")
	}
}

// TestChannelDispatchPriorityOrder 验证优先级数值越小越优先。
func TestChannelDispatchPriorityOrder(t *testing.T) {
	if !(channelPriorityManualUpload < channelPriorityRealtimeCollect &&
		channelPriorityRealtimeCollect < channelPriorityAIRepair &&
		channelPriorityAIRepair < channelPriorityCycle &&
		channelPriorityCycle < channelPriorityBatch) {
		t.Fatal("send priority order is invalid")
	}
	if !(channelPriorityManualDelete < channelPriorityKeywordDelete &&
		channelPriorityKeywordDelete < channelPriorityCycleDelete) {
		t.Fatal("delete priority order is invalid")
	}
}
