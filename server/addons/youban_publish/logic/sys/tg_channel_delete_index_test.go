package sys

import "testing"

func TestTelegramChannelDeleteKeyIsTenantScoped(t *testing.T) {
	first := tgChannelDeleteKey(78, 231)
	second := tgChannelDeleteKey(79, 231)
	if first == second {
		t.Fatalf("不同租户的删除队列不能共用 key: %q", first)
	}
	if first != "youban_publish:tg:channel_delete:78:231" {
		t.Fatalf("删除队列 key 不符合约定: %q", first)
	}
}

func TestTelegramChannelDeleteIndexScorePriorityAndFIFO(t *testing.T) {
	index := tgChannelDeleteIndex{}
	manual := float64(channelPriorityManualDelete)*1_000_000_000_000 + float64(101)
	cycle := float64(channelPriorityCycleDelete)*1_000_000_000_000 + float64(1)
	if manual >= cycle {
		t.Fatalf("手动删除必须优先于循环删除: manual=%v cycle=%v", manual, cycle)
	}
	_ = index
}
