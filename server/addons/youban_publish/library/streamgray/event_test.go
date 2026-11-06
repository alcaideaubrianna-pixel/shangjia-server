package streamgray

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
)

type fakeProducer struct {
	stream string
	event  ReadyEvent
}

func (f *fakeProducer) Add(_ context.Context, stream string, event ReadyEvent) (string, error) {
	f.stream, f.event = stream, event
	return "1-0", nil
}

// TestReadyEventRoundTrip 验证就绪事件编码和解码保持幂等字段。
func TestReadyEventRoundTrip(t *testing.T) {
	want := ReadyEvent{JobID: 123, ChannelID: 456, Priority: 90, QueueName: "publish"}
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
	if _, err := (ReadyEvent{ChannelID: 1}).Encode(); err == nil {
		t.Fatal("missing job id must fail")
	}
	if _, err := DecodeReadyEvent([]byte(`{"job_id":1}`)); err == nil {
		t.Fatal("missing channel id must fail")
	}
}

// TestPublishReadyEventFacade 验证业务层只能通过允许的 Stream 发布就绪事件。
func TestPublishReadyEventFacade(t *testing.T) {
	producer := &fakeProducer{}
	event := ReadyEvent{JobID: 1, ChannelID: 2}
	id, err := PublishReadyEvent(context.Background(), producer, ReadyRealtimeStream, event)
	if err != nil || id != "1-0" || producer.stream != ReadyRealtimeStream || producer.event != event {
		t.Fatalf("publish result id=%q err=%v producer=%+v", id, err, producer)
	}
	if _, err = PublishReadyEvent(context.Background(), producer, "other", event); err == nil {
		t.Fatal("unknown stream must fail")
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
	id, err := producer.Add(context.Background(), ReadyRealtimeStream, ReadyEvent{JobID: 1, ChannelID: 2})
	if err != nil || id != "1700000000000-0" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if fake.command != "XADD" || len(fake.args) != 4 || fake.args[0] != ReadyRealtimeStream || fake.args[1] != "*" || fake.args[2] != "payload" {
		t.Fatalf("command=%q args=%v", fake.command, fake.args)
	}
}
