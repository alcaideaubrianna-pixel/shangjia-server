package streamgray

import (
	"context"
	"testing"
)

// TestProductionShapeReplay 使用 DBX 只读取得的任务形状回放 Stream 门禁。
// 仅保留租户/账户/频道/操作号/优先级，不包含正文、凭据或 Telegram 地址。
func TestProductionShapeReplay(t *testing.T) {
	fixtures := []ReadyEvent{
		{TenantID: 25, AccountID: 473, JobID: 2834248, ChannelID: 103, Priority: 10, QueueName: "collect"},
		{TenantID: 56, AccountID: 561, JobID: 2833882, ChannelID: 155, Priority: 0, QueueName: "cycle"},
		{TenantID: 56, AccountID: 561, JobID: 2834219, ChannelID: 155, Priority: 90, QueueName: "cycle"},
		{TenantID: 78, AccountID: 615, JobID: 2834216, ChannelID: 232, Priority: 90, QueueName: "cycle"},
		{TenantID: 12, AccountID: 437, JobID: 2834215, ChannelID: 121, Priority: 90, QueueName: "cycle"},
	}
	gate := &replayGate{enabled: true}
	dispatcher := &replayDispatcher{jobs: make(map[int64]int)}
	for _, event := range fixtures {
		if err := DispatchReadyEventWithGate(context.Background(), gate, dispatcher, event); err != nil {
			t.Fatal(err)
		}
	}
	if len(dispatcher.jobs) != len(fixtures) {
		t.Fatalf("production-shaped replay dispatched=%d want=%d", len(dispatcher.jobs), len(fixtures))
	}
	gate.enabled = false
	if err := DispatchReadyEventWithGate(context.Background(), gate, dispatcher, fixtures[0]); err != nil {
		t.Fatal(err)
	}
	if dispatcher.jobs[fixtures[0].JobID] != 1 {
		t.Fatal("rollback must prevent replayed production-shaped event")
	}
}
