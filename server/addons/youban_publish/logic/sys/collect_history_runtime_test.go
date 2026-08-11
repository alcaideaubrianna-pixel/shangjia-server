package sys

import (
	"context"
	"errors"
	"testing"
)

func TestCollectHistoryAccountTaskKey(t *testing.T) {
	if got, want := collectHistoryAccountTaskKey(5, 100, 200), "history:5:offset:100:version:200"; got != want {
		t.Fatalf("key = %q, want %q", got, want)
	}
}

func TestCollectHistoryNextPageLimit(t *testing.T) {
	tests := []struct {
		name         string
		pendingCount int
		pendingLimit int
		want         int
	}{
		{name: "empty backlog uses page limit", pendingCount: 0, pendingLimit: 200, want: collectHistoryPageLimit},
		{name: "remaining capacity limits page", pendingCount: 70, pendingLimit: 80, want: 10},
		{name: "full backlog stops expansion", pendingCount: 80, pendingLimit: 80, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectHistoryNextPageLimit(test.pendingCount, test.pendingLimit); got != test.want {
				t.Fatalf("page limit = %d, want %d", got, test.want)
			}
		})
	}
}

func TestCollectHistoryTransientClientError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "context canceled", err: context.Canceled, want: true},
		{name: "wrapped client closed", err: errors.New("拉取历史消息失败: client closed: context canceled"), want: true},
		{name: "dc closed", err: errors.New("rpcDoRequest: DC is closed"), want: true},
		{name: "business error", err: errors.New("频道AccessHash无效"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectHistoryTransientClientError(test.err); got != test.want {
				t.Fatalf("transient = %v, want %v", got, test.want)
			}
		})
	}
}

func TestAccountCollectWorkerConfigHotUpdate(t *testing.T) {
	worker := &accountCollectWorker{
		signature: "old",
	}
	changed := worker.updateConfig("new", []accountListenPlanRuntime{{Id: 30}})
	if !changed {
		t.Fatal("config update must report a change")
	}
	listeners := worker.configSnapshot()
	if len(listeners) != 1 || listeners[0].Id != 30 {
		t.Fatalf("listeners = %+v, want listener 30", listeners)
	}
	if worker.updateConfig("new", listeners) {
		t.Fatal("same signature must not trigger another update")
	}
}
