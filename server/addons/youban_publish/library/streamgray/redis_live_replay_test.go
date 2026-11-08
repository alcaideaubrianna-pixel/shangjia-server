package streamgray

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	red "github.com/go-redis/redis/v8"
	"github.com/gogf/gf/v2/container/gvar"
)

type goRedisCommandAdapter struct{ client *red.Client }

func (a goRedisCommandAdapter) Do(ctx context.Context, command string, args ...any) (*gvar.Var, error) {
	values := make([]interface{}, 0, len(args))
	for _, arg := range args {
		values = append(values, arg)
	}
	cmd := a.client.Do(ctx, append([]interface{}{command}, values...)...)
	value, err := cmd.Result()
	if err != nil {
		return nil, err
	}
	return gvar.New(value), nil
}

func TestRedisLiveReplayWithMiniredis(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	client := red.NewClient(&red.Options{Addr: server.Addr()})
	defer client.Close()
	adapter := goRedisCommandAdapter{client: client}
	producer, err := NewRedisProducer(adapter)
	if err != nil {
		t.Fatal(err)
	}
	event := ReadyEvent{TenantID: 73, AccountID: 608, JobID: 9001, ChannelID: 231}
	groupClient, err := NewRedisCommandGroupClientDefault(adapter)
	if err != nil {
		t.Fatal(err)
	}
	if err = groupClient.CreateGroup(context.Background(), ReadyRealtimeStream, "replay"); err != nil {
		t.Fatal(err)
	}
	if _, err = producer.Add(context.Background(), ReadyRealtimeStream, event); err != nil {
		t.Fatal(err)
	}
	consumer, err := NewRedisGroupConsumer(groupClient, "replay", "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan ReadyEvent, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		_ = consumer.Run(ctx, ReadyRealtimeStream, func(_ context.Context, value ReadyEvent) error {
			received <- value
			cancel()
			return nil
		})
	}()
	select {
	case got := <-received:
		if got != event {
			t.Fatalf("live replay event=%v want=%v", got, event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("live replay timed out")
	}
}
