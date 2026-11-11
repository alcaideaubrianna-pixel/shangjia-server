package streamgray

import (
	"context"
	"errors"
	"os"
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

// TestRedisLiveChannelIsolation 在专用本地 Redis 上验证忙频道保留 pending、其他频道确认且无丢失。
func TestRedisLiveChannelIsolation(t *testing.T) {
	address := os.Getenv("YOUBAN_STREAM_REDIS_TEST_ADDR")
	if address != "127.0.0.1:16380" {
		t.Skip("requires dedicated disposable Redis on 127.0.0.1:16380")
	}
	client := red.NewClient(&red.Options{Addr: address})
	defer client.Close()
	adapter := goRedisCommandAdapter{client: client}
	producer, err := NewRedisProducer(adapter)
	if err != nil {
		t.Fatal(err)
	}
	groupClient, err := NewRedisCommandGroupClientDefault(adapter)
	if err != nil {
		t.Fatal(err)
	}
	group := "isolation-" + time.Now().Format("150405.000000000")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 该固定端口只能由专用、可销毁的测试容器提供；避免上次测试的事件污染本次计数。
	if err := client.Del(ctx, ReadyRealtimeStream).Err(); err != nil {
		t.Fatal(err)
	}
	defer client.Del(context.Background(), ReadyRealtimeStream)
	if err := groupClient.CreateGroup(ctx, ReadyRealtimeStream, group); err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 200; id++ {
		if _, err := producer.Add(ctx, ReadyRealtimeStream, ReadyEvent{TenantID: 1, AccountID: 1, JobID: id, ChannelID: id}); err != nil {
			t.Fatal(err)
		}
	}
	consumer, err := NewRedisGroupConsumer(groupClient, group, "isolation-worker")
	if err != nil {
		t.Fatal(err)
	}
	consumer.block = 50 * time.Millisecond
	received := make(chan int64, 200)
	done := make(chan error, 1)
	started := time.Now()
	go func() {
		done <- consumer.Run(ctx, ReadyRealtimeStream, func(_ context.Context, event ReadyEvent) error {
			if event.ChannelID == 1 {
				return errors.New("lock failed")
			}
			received <- event.JobID
			return nil
		})
	}()
	seen := map[int64]bool{}
	for len(seen) < 199 {
		select {
		case id := <-received:
			if seen[id] {
				t.Fatalf("duplicate dispatch %d", id)
			}
			seen[id] = true
		case err := <-done:
			t.Fatalf("consumer exited early: %v", err)
		case <-ctx.Done():
			t.Fatalf("healthy channels blocked: received=%d", len(seen))
		}
	}
	for {
		pending, err := client.XPending(ctx, ReadyRealtimeStream, group).Result()
		if err != nil {
			t.Fatal(err)
		}
		if pending.Count == 1 && pending.Lower == pending.Higher {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("pending not drained: %+v", pending)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("consumer did not stop")
	}
	t.Logf("199 healthy channels acknowledged with 1 busy channel in %s", time.Since(started))
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
