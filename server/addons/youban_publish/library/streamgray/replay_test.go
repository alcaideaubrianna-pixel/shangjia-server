package streamgray

import (
	"context"
	"testing"
)

// replayGate 用于回放测试中模拟账户灰度配置，可在中途回滚账户。
type replayGate struct{ enabled bool }

func (g *replayGate) Enabled(context.Context, int64, int64) (bool, error) { return g.enabled, nil }

type replayDispatcher struct{ jobs map[int64]int }

func (d *replayDispatcher) Dispatch(_ context.Context, jobID int64) error {
	d.jobs[jobID]++
	return nil
}

func TestReplayEventsAndRollback(t *testing.T) {
	gate := &replayGate{enabled: true}
	dispatcher := &replayDispatcher{jobs: make(map[int64]int)}
	for i := int64(1); i <= 100; i++ {
		event := ReadyEvent{TenantID: 73, AccountID: 608, JobID: i, ChannelID: 900 + i}
		if err := DispatchReadyEventWithGate(context.Background(), gate, dispatcher, event); err != nil {
			t.Fatal(err)
		}
		// 模拟 Redis/Consumer 重复投递；最终 Asynq 适配器必须自行幂等。
		if err := DispatchReadyEventWithGate(context.Background(), gate, dispatcher, event); err != nil {
			t.Fatal(err)
		}
	}
	if len(dispatcher.jobs) != 100 || dispatcher.jobs[1] != 2 {
		t.Fatalf("replayed jobs = %d first count=%d", len(dispatcher.jobs), dispatcher.jobs[1])
	}
	gate.enabled = false
	if err := DispatchReadyEventWithGate(context.Background(), gate, dispatcher, ReadyEvent{TenantID: 73, AccountID: 608, JobID: 101, ChannelID: 1001}); err != nil {
		t.Fatal(err)
	}
	if dispatcher.jobs[101] != 0 {
		t.Fatal("rolled back account must not dispatch historical event")
	}
}
