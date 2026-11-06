package streamgray

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
)

type fakeGroupClient struct {
	entries []StreamEntry
	acked   []string
}

type emptyGroupClient struct{}

func (emptyGroupClient) ReadGroup(context.Context, string, string, string, string, int, time.Duration) ([]StreamEntry, error) {
	return nil, nil
}

func (emptyGroupClient) Ack(context.Context, string, string, string) error { return nil }

type adminGroupClient struct {
	mu     sync.Mutex
	groups []string
}

func (a *adminGroupClient) ReadGroup(context.Context, string, string, string, string, int, time.Duration) ([]StreamEntry, error) {
	return nil, nil
}
func (a *adminGroupClient) Ack(context.Context, string, string, string) error { return nil }
func (a *adminGroupClient) CreateGroup(_ context.Context, stream, group string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.groups = append(a.groups, stream+":"+group)
	return nil
}

func (f *fakeGroupClient) ReadGroup(_ context.Context, _, _, _, _ string, _ int, _ time.Duration) ([]StreamEntry, error) {
	return f.entries, nil
}

func (f *fakeGroupClient) Ack(_ context.Context, _, _, entryID string) error {
	f.acked = append(f.acked, entryID)
	return nil
}

type fakeProducer struct {
	stream string
	event  ReadyEvent
}

type fakeDispatcher struct {
	jobIDs []int64
}

type fakeAccountGate struct{ enabled bool }

func (f *fakeAccountGate) Enabled(context.Context, int64, int64) (bool, error) { return f.enabled, nil }

func (f *fakeDispatcher) Dispatch(_ context.Context, jobID int64) error {
	f.jobIDs = append(f.jobIDs, jobID)
	return nil
}

func (f *fakeProducer) Add(_ context.Context, stream string, event ReadyEvent) (string, error) {
	f.stream, f.event = stream, event
	return "1-0", nil
}

// TestReadyEventRoundTrip 验证就绪事件编码和解码保持幂等字段。
func TestReadyEventRoundTrip(t *testing.T) {
	want := ReadyEvent{TenantID: 10, AccountID: 20, JobID: 123, ChannelID: 456, Priority: 90, QueueName: "publish"}
	body, err := want.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeReadyEvent(body)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("event = %+v, want %+v", got, want)
	}
}

// TestReadyEventValidation 验证无效事件不会进入传输适配层。
func TestReadyEventValidation(t *testing.T) {
	if _, err := (ReadyEvent{JobID: 1, ChannelID: 1}).Encode(); err == nil {
		t.Fatal("missing job id must fail")
	}
	if _, err := DecodeReadyEvent([]byte(`{"tenant_id":1,"account_id":1,"job_id":1}`)); err == nil {
		t.Fatal("missing channel id must fail")
	}
}

// TestStreamForOperation 验证循环和全量任务进入 bulk 流，其他任务进入 realtime 流。
func TestStreamForOperation(t *testing.T) {
	if got := StreamForOperation("cycle_batch:1"); got != ReadyBulkStream {
		t.Fatalf("cycle stream = %q", got)
	}
	if got := StreamForOperation("full_push:1"); got != ReadyBulkStream {
		t.Fatalf("full push stream = %q", got)
	}
	if got := StreamForOperation("collect:1"); got != ReadyRealtimeStream {
		t.Fatalf("collect stream = %q", got)
	}
}

// TestPublishReadyEventFacade 验证业务层只能通过允许的 Stream 发布就绪事件。
func TestPublishReadyEventFacade(t *testing.T) {
	producer := &fakeProducer{}
	event := ReadyEvent{TenantID: 1, AccountID: 2, JobID: 1, ChannelID: 2}
	id, err := PublishReadyEvent(context.Background(), producer, ReadyRealtimeStream, event)
	if err != nil || id != "1-0" || producer.stream != ReadyRealtimeStream || producer.event != event {
		t.Fatalf("publish result id=%q err=%v producer=%+v", id, err, producer)
	}
	if _, err = PublishReadyEvent(context.Background(), producer, "other", event); err == nil {
		t.Fatal("unknown stream must fail")
	}
}

// TestPublishAfterCommit 验证数据库提交失败时不会产生 Stream 幽灵消息。
func TestPublishAfterCommit(t *testing.T) {
	producer := &fakeProducer{}
	called := false
	if _, err := PublishAfterCommit(context.Background(), func(context.Context) error {
		called = true
		return context.Canceled
	}, producer, ReadyRealtimeStream, ReadyEvent{TenantID: 1, AccountID: 2, JobID: 1, ChannelID: 2}); err != context.Canceled {
		t.Fatalf("commit err = %v", err)
	}
	if !called || producer.stream != "" {
		t.Fatalf("commit failure must not publish: called=%t producer=%+v", called, producer)
	}
	if _, err := PublishAfterCommit(context.Background(), func(context.Context) error { return nil }, producer, ReadyRealtimeStream, ReadyEvent{TenantID: 1, AccountID: 2, JobID: 1, ChannelID: 2}); err != nil {
		t.Fatal(err)
	}
}

// TestDispatchReadyEvent 验证 Stream 只调用现有任务调度器，不直接执行发送。
func TestDispatchReadyEvent(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	event := ReadyEvent{TenantID: 1, AccountID: 2, JobID: 7, ChannelID: 9}
	if err := DispatchReadyEvent(context.Background(), dispatcher, event); err != nil {
		t.Fatal(err)
	}
	if len(dispatcher.jobIDs) != 1 || dispatcher.jobIDs[0] != event.JobID {
		t.Fatalf("dispatched jobs = %v", dispatcher.jobIDs)
	}
	if err := DispatchReadyEvent(context.Background(), dispatcher, ReadyEvent{}); err == nil {
		t.Fatal("invalid event must fail")
	}
}

func TestDispatchReadyEventWithGateSkipsRolledBackAccount(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	event := ReadyEvent{TenantID: 1, AccountID: 2, JobID: 7, ChannelID: 9}
	if err := DispatchReadyEventWithGate(context.Background(), &fakeAccountGate{}, dispatcher, event); err != nil {
		t.Fatal(err)
	}
	if len(dispatcher.jobIDs) != 0 {
		t.Fatalf("rolled back account must not dispatch: %v", dispatcher.jobIDs)
	}
	if err := DispatchReadyEventWithGate(context.Background(), &fakeAccountGate{enabled: true}, dispatcher, event); err != nil {
		t.Fatal(err)
	}
	if len(dispatcher.jobIDs) != 1 {
		t.Fatalf("enabled account must dispatch: %v", dispatcher.jobIDs)
	}
}

func TestNewGatedHandler(t *testing.T) {
	dispatcher := &fakeDispatcher{}
	handler := NewGatedHandler(&fakeAccountGate{enabled: true}, dispatcher)
	if err := handler(context.Background(), ReadyEvent{TenantID: 1, AccountID: 2, JobID: 8, ChannelID: 9}); err != nil {
		t.Fatal(err)
	}
	if len(dispatcher.jobIDs) != 1 || dispatcher.jobIDs[0] != 8 {
		t.Fatalf("jobs = %v", dispatcher.jobIDs)
	}
}

type fakeRedisClient struct {
	command string
	args    []any
}

func (f *fakeRedisClient) Do(_ context.Context, command string, args ...any) (*gvar.Var, error) {
	f.command, f.args = command, args
	return gvar.New("1700000000000-0"), nil
}

// TestRedisProducerXAdd 验证 Redis 适配器只通过 XADD 写入事件。
func TestRedisProducerXAdd(t *testing.T) {
	fake := &fakeRedisClient{}
	producer, err := NewRedisProducer(fake)
	if err != nil {
		t.Fatal(err)
	}
	id, err := producer.Add(context.Background(), ReadyRealtimeStream, ReadyEvent{TenantID: 1, AccountID: 2, JobID: 1, ChannelID: 2})
	if err != nil || id != "1700000000000-0" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if fake.command != "XADD" || len(fake.args) != 4 || fake.args[0] != ReadyRealtimeStream || fake.args[1] != "*" || fake.args[2] != "payload" {
		t.Fatalf("command=%q args=%v", fake.command, fake.args)
	}
}

// TestRedisGroupConsumerAcksAfterHandler 验证只有 handler 成功后才确认消息。
func TestRedisGroupConsumerAcksAfterHandler(t *testing.T) {
	body, err := (ReadyEvent{TenantID: 1, AccountID: 2, JobID: 1, ChannelID: 2}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeGroupClient{entries: []StreamEntry{{ID: "1-0", Payload: body}}}
	consumer, err := NewRedisGroupConsumer(fake, "publish", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	processed, err := consumer.ConsumeOnce(context.Background(), ReadyRealtimeStream, ">", func(_ context.Context, event ReadyEvent) error {
		if event.JobID != 1 {
			t.Fatalf("event = %+v", event)
		}
		return nil
	})
	if err != nil || processed != 1 || len(fake.acked) != 1 || fake.acked[0] != "1-0" {
		t.Fatalf("processed=%d err=%v acked=%v", processed, err, fake.acked)
	}
}

// TestRedisGroupConsumerKeepsFailedEntryPending 验证 handler 失败时绝不 ACK，便于重试或恢复消费者接管。
func TestRedisGroupConsumerKeepsFailedEntryPending(t *testing.T) {
	body, err := (ReadyEvent{TenantID: 1, AccountID: 2, JobID: 2, ChannelID: 3}).Encode()
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeGroupClient{entries: []StreamEntry{{ID: "2-0", Payload: body}}}
	consumer, err := NewRedisGroupConsumer(fake, "publish", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = consumer.ConsumeOnce(context.Background(), ReadyRealtimeStream, ">", func(context.Context, ReadyEvent) error {
		return context.Canceled
	}); err != context.Canceled {
		t.Fatalf("handler err = %v", err)
	}
	if len(fake.acked) != 0 {
		t.Fatalf("failed entry must remain pending, acked=%v", fake.acked)
	}
}

func TestRedisGroupConsumerRunStopsWithContext(t *testing.T) {
	consumer, err := NewRedisGroupConsumer(emptyGroupClient{}, "publish", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = consumer.Run(ctx, ReadyRealtimeStream, func(context.Context, ReadyEvent) error { return nil })
	if err != context.Canceled {
		t.Fatalf("run err = %v, want %v", err, context.Canceled)
	}
}

func TestRunBothStreamsStopsWithContext(t *testing.T) {
	consumer, err := NewRedisGroupConsumer(emptyGroupClient{}, "publish", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = RunBothStreams(ctx, consumer, func(context.Context, ReadyEvent) error { return nil })
	if err != context.Canceled {
		t.Fatalf("run both err = %v, want %v", err, context.Canceled)
	}
}

func TestRunBothStreamsInitializesBothGroups(t *testing.T) {
	client := &adminGroupClient{}
	consumer, err := NewRedisGroupConsumer(client, "publish", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	if err = RunBothStreams(ctx, consumer, func(context.Context, ReadyEvent) error { return nil }); err != context.Canceled {
		t.Fatalf("run err = %v", err)
	}
	if len(client.groups) != 2 {
		t.Fatalf("groups initialized = %v", client.groups)
	}
}
