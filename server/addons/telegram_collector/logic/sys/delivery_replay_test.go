package sys

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type deliveryReplayFixture struct {
	ID         int64           `json:"id"`
	TenantID   int64           `json:"tenant_id"`
	SourceID   int64           `json:"source_id"`
	SourceType string          `json:"source_type"`
	AccountID  int64           `json:"account_id"`
	ChatID     string          `json:"chat_id"`
	MessageID  int64           `json:"message_id"`
	Priority   int             `json:"priority"`
	Raw        json.RawMessage `json:"raw_update"`
	CreatedAt  *gtime.Time     `json:"created_at"`
	ReceivedAt *gtime.Time     `json:"received_at"`
}

type deliveryReplaySink struct {
	expected map[int64][32]byte
	mu       sync.Mutex
	calls    map[int64]int
	attempts map[int64]int
	failedID int64
	blocked  chan struct{}
	entered  chan struct{}
	once     sync.Once
	latency  map[int64]time.Duration
	start    time.Time
	realtime map[int64]bool
}

func (s *deliveryReplaySink) HandleCollectorDelivery(ctx context.Context, d *sysin.CollectorDelivery) error {
	if d.History {
		s.once.Do(func() { close(s.entered) })
		select {
		case <-s.blocked:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if deliveryReplaySignature(d) != s.expected[d.ID] {
		return fmt.Errorf("replayed delivery metadata differs from production fixture")
	}
	s.attempts[d.ID]++
	if d.ID == s.failedID && s.attempts[d.ID] == 1 {
		return fmt.Errorf("injected temporary downstream failure")
	}
	s.calls[d.ID]++
	if s.realtime[d.ID] {
		s.latency[d.ID] = time.Since(s.start)
	}
	return nil
}

// Offline replay only: every database connection is explicitly loopback and
// the downstream sink cannot download media, publish Telegram, or write
// profiles. Production fixture content is never printed or checked in.
func TestDeliveryProductionFixtureReplay(t *testing.T) {
	path := os.Getenv("YOUBAN_DELIVERY_REPLAY_FILE")
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if path == "" || port == "" {
		t.Skip("requires private fixture and disposable loopback PostgreSQL")
	}
	oldDB := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", oldDB)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres", MaxOpenConnCount: 12}})
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	cfg := g.Cfg().GetAdapter()
	defer g.Cfg().SetAdapter(cfg)
	adapter, err := gcfg.NewAdapterContent(fmt.Sprintf("telegramCollector:\n  enabled: true\n  delivery:\n    concurrency: 8\n    historyIntervalMilliseconds: 100\nredis:\n  default:\n    address: '%s'\n", redis.Addr()))
	if err != nil {
		t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
	}
	g.Cfg().SetAdapter(adapter)
	ctx := context.Background()
	if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal("clear disposable schema cache failed")
	}
	defer g.DB().GetCore().ClearTableFieldsAll(ctx)
	ddl, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
	}
	if _, err = g.DB().Exec(ctx, string(ddl)); err != nil {
		t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
	}
	// All destructive preparation is confined to the explicitly disposable DB.
	if _, err = g.DB().Exec(ctx, "TRUNCATE hg_tg_collector_delivery,hg_tg_collector_event RESTART IDENTITY"); err != nil {
		t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 65536), 8*1024*1024)
	var rows []deliveryReplayFixture
	for scanner.Scan() {
		var f deliveryReplayFixture
		if err = json.Unmarshal(scanner.Bytes(), &f); err != nil {
			t.Fatal("invalid fixture row", err)
		}
		rows = append(rows, f)
	}
	if err = scanner.Err(); err != nil {
		t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
	}
	if len(rows) < 1000 {
		t.Fatal("replay requires at least 1000 production samples")
	}
	client := asynq.NewClient(opt)
	collector := &sCollector{queueClient: client}
	sink := &deliveryReplaySink{expected: map[int64][32]byte{}, calls: map[int64]int{}, attempts: map[int64]int{}, realtime: map[int64]bool{}, latency: map[int64]time.Duration{}, blocked: make(chan struct{}), entered: make(chan struct{})}
	previous := collectorservice.CollectorDeliveryHandler()
	collectorservice.RegisterDeliveryHandler(sink)
	defer collectorservice.RegisterDeliveryHandler(previous)
	accounts := map[int64]bool{}
	sources := map[int64]bool{}
	history, live, media, groups := 0, 0, 0, 0
	deliveryIDs := make([]int64, 0, len(rows))
	for _, f := range rows {
		raw := &sysin.RawUpdateEvent{EventID: fmt.Sprintf("replay:%d", f.ID), TenantID: f.TenantID, SourceID: f.SourceID, SourceType: f.SourceType, AccountID: f.AccountID, ChatID: parseCollectorChatID(f.ChatID), MessageID: f.MessageID, RawUpdate: []byte(f.Raw), Priority: f.Priority}
		if f.ReceivedAt != nil {
			raw.ReceivedAt = f.ReceivedAt.Time
		}
		id, err := collector.persistEvent(ctx, raw)
		if err != nil {
			t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
		}
		if f.CreatedAt != nil {
			if _, err = g.DB().Exec(ctx, "UPDATE hg_tg_collector_event SET created_at=? WHERE id=?", f.CreatedAt, id); err != nil {
				t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
			}
		}
		event := &entity.TgCollectorEvent{Id: id, TenantId: f.TenantID, SourceId: f.SourceID, ChatId: f.ChatID, MessageId: f.MessageID, SourceType: f.SourceType, RawUpdate: gjson.New(f.Raw), CreatedAt: f.CreatedAt, ReceivedAt: f.ReceivedAt}
		d, err := collectorDeliveryFromEvent(event, []byte(f.Raw))
		if err != nil {
			t.Fatal("fixture classification failed", err)
		}
		if d.History {
			history++
		} else {
			live++
		}
		if len(d.Media) > 0 {
			media++
		}
		if d.SourceGroupedID != "" {
			groups++
		}
		if f.AccountID > 0 {
			accounts[f.AccountID] = true
		}
		sources[f.SourceID] = true
		if err = collector.processEvent(ctx, id); err != nil {
			t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
		}
		rec, err := g.DB().GetOne(ctx, "SELECT id FROM hg_tg_collector_delivery WHERE event_id=?", id)
		if err != nil {
			t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
		}
		deliveryID := rec["id"].Int64()
		sink.expected[deliveryID] = deliveryReplaySignature(&d)
		deliveryIDs = append(deliveryIDs, deliveryID)
		if !d.History {
			sink.realtime[deliveryID] = true
		}
		if sink.failedID == 0 && !d.History {
			sink.failedID = deliveryID
		}
		// Model both duplicate ingress and pre-upgrade Redis jobs. Conditional
		// claim must prevent the two queues from calling the business sink twice.
		if id%50 == 0 {
			if err = collector.processEvent(ctx, id); err != nil {
				t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
			}
			if _, err = client.Enqueue(asynq.NewTask(deliveryTaskType, collectorTaskPayload(deliveryID)), asynq.Queue(consts.QueueDeliveryReady)); err != nil {
				t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
			}
		}
	}
	if history == 0 || live == 0 || len(accounts) < 2 {
		t.Fatal("fixture lacks workload diversity")
	}
	sink.start = time.Now()
	collector.StartDeliveryRuntime(ctx)
	stop := true
	defer func() {
		if stop {
			close(sink.blocked)
			collector.StopRuntime()
		}
	}()
	select {
	case <-sink.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("history worker did not start")
	}
	// History is deliberately stuck. Realtime and legacy classification must
	// continue independently; a shared worker would fail this test.
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		sink.mu.Lock()
		done := len(sink.latency)
		sink.mu.Unlock()
		if done >= live-1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	sink.mu.Lock()
	done := len(sink.latency)
	latencies := make([]time.Duration, 0, done)
	for _, v := range sink.latency {
		latencies = append(latencies, v)
	}
	sink.mu.Unlock()
	if done < live-1 {
		t.Fatalf("blocked history starved realtime: %d/%d", done, live)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := latencies[(len(latencies)-1)*95/100]
	close(sink.blocked)
	collector.StopRuntime()
	stop = false
	// Drain the remaining historical payloads directly to check reconstruction
	// and idempotence for the entire fixture without waiting out a multi-hour
	// production rate limit. Failure retry is exercised on the same delivery ID.
	for _, id := range deliveryIDs {
		if _, err = g.DB().Exec(ctx, "UPDATE hg_tg_collector_delivery SET status='pending' WHERE id=? AND status='failed_retry'", id); err != nil {
			t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
		}
		if err = collector.processDelivery(ctx, id); err != nil {
			t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
		}
		if err = collector.processDelivery(ctx, id); err != nil {
			t.Fatal("offline replay operation failed (details suppressed to protect production payload)")
		}
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.calls) != len(rows) {
		t.Fatalf("lost delivery: completed=%d fixture=%d", len(sink.calls), len(rows))
	}
	for _, n := range sink.calls {
		if n != 1 {
			t.Fatal("duplicate business callback", n)
		}
	}
	if sink.attempts[sink.failedID] != 2 {
		t.Fatal("failed delivery was not recovered")
	}
	t.Logf("production replay rows=%d accounts=%d sources=%d realtime=%d history=%d media=%d grouped=%d queue_to_sink_p95=%s duplicates=0 lost=0", len(rows), len(accounts), len(sources), live, history, media, groups, p95)
}

func deliveryReplaySignature(d *sysin.CollectorDelivery) [32]byte {
	copy := *d
	copy.ID = 0
	copy.DeliveryKey = ""
	copy.RawUpdate = nil
	body, _ := json.Marshal(copy)
	return sha256.Sum256(body)
}
