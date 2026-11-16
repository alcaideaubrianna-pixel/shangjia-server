package sys

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/hibiken/asynq"
)

func TestCollectSourceWorkerEmptyHistoryAndRestartAfterStop(t *testing.T) {
	r := miniredis.RunT(t)
	previous := g.Cfg().GetAdapter()
	previousRedis, _ := gredis.GetConfig()
	t.Cleanup(func() { g.Cfg().SetAdapter(previous); gredis.SetConfig(previousRedis) })
	adapter, err := gcfg.NewAdapterContent(fmt.Sprintf(`{"cache":{"adapter":"redis"},"redis":{"default":{"address":%q}},"youbanPublish":{"collect":{"sourceLeaseSeconds":10}}}`, r.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	gredis.SetConfig(&gredis.Config{Address: r.Addr()})
	s := &sSysPublish{collectSourceQueueServers: make(map[string]*asynq.Server)}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	queue := "test_source_history_lifecycle"
	s.ensureCollectSourceQueueWorker(ctx, queue)
	if len(s.collectSourceQueueServers) != 0 {
		t.Fatal("empty history lane allocated a server")
	}
	// A future scheduled task needs the server's existing delayed-task promoter;
	// unlike completed/archived rows it must keep a lane recoverable.
	r.ZAdd("asynq:{"+queue+"}:scheduled", float64(time.Now().Add(time.Hour).Unix()), "future")
	s.ensureCollectSourceQueueWorker(ctx, queue)
	s.tgQueueMu.Lock()
	first := s.collectSourceQueueServers[queue]
	s.tgQueueMu.Unlock()
	if first == nil {
		t.Fatal("scheduled queue failed to start")
	}
	stopCollectSourceQueueWorker(first)
	s.tgQueueMu.Lock()
	_, exists := s.collectSourceQueueServers[queue]
	s.tgQueueMu.Unlock()
	if exists {
		t.Fatal("stopped server retained in local registry")
	}
	s.ensureCollectSourceQueueWorker(ctx, queue)
	s.tgQueueMu.Lock()
	second := s.collectSourceQueueServers[queue]
	s.tgQueueMu.Unlock()
	if second == nil || second == first {
		t.Fatal("source could not restart after retirement")
	}
	stopCollectSourceQueueWorker(second)
}
