package sys

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"testing"
)

// TestLegacyPublishDrainMigrationPolicy 验证迁移默认排空旧队列，显式关闭仅对全量 Stream 生效。
func TestLegacyPublishDrainMigrationPolicy(t *testing.T) {
	before := g.Cfg().GetAdapter()
	t.Cleanup(func() { g.Cfg().SetAdapter(before) })
	setConfig := func(content string) {
		a, err := gcfg.NewAdapterContent(content)
		if err != nil {
			t.Fatal(err)
		}
		g.Cfg().SetAdapter(a)
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	setConfig(`{}`)
	if !telegramLegacyPublishQueuesEnabled(context.Background()) {
		t.Fatal("migration must drain legacy queues by default")
	}
	setConfig(`{"youbanPublish":{"queue":{"drainLegacyQueues":false}}}`)
	if telegramLegacyPublishQueuesEnabled(context.Background()) {
		t.Fatal("explicit full-stream mode must disable old shard polling")
	}
	queues := telegramPublishExecutionQueues(context.Background())
	if queues[channelDispatchQueueManual] == 0 || queues[channelDispatchQueueSend] == 0 || queues[tgQueueNameDefault] == 0 || queues[tgQueueNameBulk] == 0 {
		t.Fatalf("unexpected execution queues: %v", queues)
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", "")
	if !telegramLegacyPublishQueuesEnabled(context.Background()) {
		t.Fatal("legacy mode must keep its workers even when drain option is false")
	}
}

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
	if got := queueNameForPushEngine(pushEngineUnifiedStream, legacy); got != channelDispatchQueueSend {
		t.Fatalf("stream queue = %q, want %q", got, channelDispatchQueueSend)
	}
}

// TestChannelDispatchPriorityOrder 验证优先级数值越小越优先。
// TestChannelDispatchPriorityOrder 验证删除执行容量按业务优先级分配；发送排序由真实SQL/Go一致性测试覆盖。
func TestChannelDispatchPriorityOrder(t *testing.T) {
	queues := telegramCleanupWorkerQueues()
	if !(queues[channelDispatchQueueDeleteManual] > queues[channelDispatchQueueDeleteKeyword] && queues[channelDispatchQueueDeleteKeyword] > queues[channelDispatchQueueDeleteCycle]) {
		t.Fatal("delete worker weights do not preserve business priorities")
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

// TestUnifiedDeleteQueuesHaveConsumers 验证新增删除 lane 均已绑定到对应 Worker。
func TestUnifiedDeleteQueuesHaveConsumers(t *testing.T) {
	cleanup := telegramCleanupWorkerQueues()
	for _, queue := range []string{tgQueueNameCleanup, channelDispatchQueueDelete, channelDispatchQueueDeleteManual} {
		if cleanup[queue] <= 0 {
			t.Fatalf("cleanup worker does not consume queue %q", queue)
		}
	}
	autoDelete := telegramAutoDeleteWorkerQueues()
	for _, queue := range []string{channelDispatchQueueDeleteKeyword, channelDispatchQueueDeleteCycle} {
		if autoDelete[queue] <= 0 {
			t.Fatalf("auto-delete worker does not consume queue %q", queue)
		}
	}
}

func TestTelegramCleanupPriorityUsesRequestContext(t *testing.T) {
	for _, tc := range []struct {
		name        string
		jobPriority int
		operation   string
		want        channelDispatchPriority
	}{
		{"interactive", tgJobPriorityInteractive, "", channelPriorityManualDelete},
		{"manual_unpublish_of_cycle_job", tgJobPriorityBulk, "unpublish:10:123", channelPriorityManualDelete},
		{"manual_delete_of_history_job", tgJobPriorityUrgent, "delete:10:123", channelPriorityManualDelete},
		{"automatic_cycle", tgJobPriorityDefault, "", channelPriorityCycleDelete},
		{"blank_operation", tgJobPriorityDefault, "  ", channelPriorityCycleDelete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := telegramCleanupPriority(tc.jobPriority, "", tc.operation)
			if got != tc.want {
				t.Fatalf("cleanup priority = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestTelegramCleanupPriorityTreatsUnpublishAsManual(t *testing.T) {
	if got := telegramCleanupPriority(tgJobPriorityBulk, "unpublish", ""); got != channelPriorityManualDelete {
		t.Fatalf("unpublish cleanup priority = %d, want manual %d", got, channelPriorityManualDelete)
	}
	if got := telegramCleanupExecutionPriority(context.Background(), telegramJobRecord{Priority: tgJobPriorityBulk, OperationType: "unpublish"}); got != channelPriorityManualDelete {
		t.Fatalf("direct unpublish cleanup priority = %d, want manual %d", got, channelPriorityManualDelete)
	}
}

func TestTelegramCleanupManualReservedSlots(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		total, reserved, want int
	}{
		{"default", 4, 1, 3},
		{"disabled", 4, 0, 4},
		{"negative", 4, -1, 4},
		{"two_reserved", 4, 2, 2},
		{"clamp_all_reserved", 4, 4, 1},
		{"clamp_excessive", 4, 100, 1},
		{"one_slot", 1, 1, 1},
		{"no_capacity", 0, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, priority := range []channelDispatchPriority{channelPriorityKeywordDelete, channelPriorityCycleDelete} {
				if got := telegramCleanupSlotLimitForPriority(priority, tc.total, tc.reserved); got != tc.want {
					t.Fatalf("priority %d limit = %d, want %d", priority, got, tc.want)
				}
			}
			if got := telegramCleanupSlotLimitForPriority(channelPriorityManualDelete, tc.total, tc.reserved); got != tc.total {
				t.Fatalf("manual limit = %d, want total capacity %d", got, tc.total)
			}
		})
	}
}

func TestTelegramCleanupManualReservedSlotsConfig(t *testing.T) {
	previous := g.Cfg().GetAdapter()
	t.Cleanup(func() { g.Cfg().SetAdapter(previous) })
	for _, tc := range []struct {
		config string
		want   int
	}{
		{`{}`, 3},
		{`{"youbanPublish":{"queue":{"cleanupManualReservedSlots":0}}}`, 4},
		{`{"youbanPublish":{"queue":{"cleanupManualReservedSlots":2}}}`, 2},
		{`{"youbanPublish":{"queue":{"cleanupManualReservedSlots":99}}}`, 1},
	} {
		adapter, err := gcfg.NewAdapterContent(tc.config)
		if err != nil {
			t.Fatal(err)
		}
		g.Cfg().SetAdapter(adapter)
		if got := telegramCleanupSlotLimit(context.Background(), channelPriorityCycleDelete); got != tc.want {
			t.Fatalf("config %s: cycle limit = %d, want %d", tc.config, got, tc.want)
		}
		if got := telegramCleanupSlotLimit(context.Background(), channelPriorityManualDelete); got != 4 {
			t.Fatalf("manual capacity changed: %d", got)
		}
	}
}
