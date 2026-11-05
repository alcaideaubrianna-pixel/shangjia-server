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

// TestQueueNameForPushEngine 验证灰度开关不会改变旧账户的队列路径。
func TestQueueNameForPushEngine(t *testing.T) {
	legacy := "youban_publish_tg_3"
	if got := queueNameForPushEngine(pushEngineLegacy, legacy); got != legacy {
		t.Fatalf("legacy queue = %q, want %q", got, legacy)
	}
	if got := queueNameForPushEngine(pushEngineUnified, legacy); got != channelDispatchQueueSend {
		t.Fatalf("unified queue = %q, want %q", got, channelDispatchQueueSend)
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

// TestChannelDispatchDeleteQueueName 验证删除任务按业务优先级进入独立队列。
func TestChannelDispatchDeleteQueueName(t *testing.T) {
	cases := []struct {
		priority channelDispatchPriority
		want     string
	}{
		{channelPriorityManualDelete, channelDispatchQueueDeleteManual},
		{channelPriorityKeywordDelete, channelDispatchQueueDeleteKeyword},
		{channelPriorityCycleDelete, channelDispatchQueueDeleteCycle},
	}
	for _, item := range cases {
		if got := channelDispatchDeleteQueueName(item.priority); got != item.want {
			t.Fatalf("priority %d queue = %q, want %q", item.priority, got, item.want)
		}
	}
}
