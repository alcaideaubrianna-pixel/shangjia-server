package sys

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/cache"
)

func TestDeliveryRecoveryPriorityLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_tg_collector_event(id bigint,source_type text,raw_update jsonb,created_at timestamp,received_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_tg_collector_delivery(id bigint PRIMARY KEY,event_id bigint,priority int) ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_event(id,source_type,raw_update,created_at) VALUES(1,'account','{"history":true}',now()),(2,'account','{"history":false}',now()),(3,'account','{"receivedAt":"2020-01-01T00:00:00Z"}',now())`,
			`INSERT INTO hg_tg_collector_event VALUES(4,'account','{"receivedAt":"2026-11-02T22:12:00+08:00"}','2026-11-02 22:12:01','2026-11-02 22:12:00'),(5,'account','{"receivedAt":"2026-11-01T22:12:00+08:00"}','2026-11-02 22:12:01','2026-11-01 22:12:00')`,
			`INSERT INTO hg_tg_collector_delivery VALUES(11,1,0),(12,2,0),(13,3,0),(14,4,50),(15,5,0)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		for id, want := range map[int64]int{11: sysin.EventPriorityNormal, 12: sysin.EventPriorityRealtime, 13: sysin.EventPriorityNormal, 14: 50, 15: sysin.EventPriorityNormal} {
			got, err := NewCollector().persistedDeliveryPriority(ctx, id)
			if err != nil {
				return err
			}
			if got != want {
				t.Fatalf("delivery %d recovery priority=%d want=%d", id, got, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeliveryHistoryAndRealtimeClassification(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		name, raw string
		want      string
	}{
		{"explicit history even recent", `{"history":true}`, consts.QueueDeliveryHistory},
		{"explicit realtime even old message", `{"history":false,"receivedAt":"2020-01-01T00:00:00Z"}`, consts.QueueDeliveryRealtime},
		{"legacy history", `{"receivedAt":"2020-01-01T00:00:00Z"}`, consts.QueueDeliveryHistory},
		{"legacy realtime", `{"receivedAt":"` + now.Format(time.RFC3339) + `"}`, consts.QueueDeliveryRealtime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := &entity.TgCollectorEvent{SourceType: sysin.SourceTypeAccount, CreatedAt: gtime.New(now)}
			delivery, err := collectorDeliveryFromEvent(event, []byte(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got := deliveryQueueName(context.Background(), 0, deliveryPriority(&delivery)); got != tc.want {
				t.Fatalf("queue=%s want=%s", got, tc.want)
			}
			// Recovery must not classify against the wall clock after a long backlog.
			again, err := collectorDeliveryFromEvent(event, []byte(tc.raw))
			if err != nil || deliveryPriority(&again) != deliveryPriority(&delivery) {
				t.Fatal("recovery changed priority", err)
			}
		})
	}
	for _, history := range []bool{false, true} {
		message := sysin.AccountMessageEvent{History: history, ReceivedAt: now}
		raw, _ := json.Marshal(message)
		delivery, err := collectorDeliveryFromEvent(&entity.TgCollectorEvent{SourceType: sysin.SourceTypeAccount, CreatedAt: gtime.New(now)}, raw)
		if err != nil || accountMessagePriority(&message) != deliveryPriority(&delivery) {
			t.Fatal("ingress and delivery disagree", err)
		}
	}
}

func TestDeliveryRedisQueueIsolationAndDuplicateEnqueue(t *testing.T) {
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	collector := &sCollector{queueClient: client}
	ctx := context.Background()
	for i := int64(1); i <= 20; i++ {
		if err := collector.enqueueDeliveryTask(ctx, i, sysin.EventPriorityNormal); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []int{sysin.EventPriorityRealtime, sysin.EventPriorityUrgent} {
		for i := 0; i < 3; i++ {
			if err := collector.enqueueDeliveryTask(ctx, int64(p), p); err != nil {
				t.Fatal(err)
			}
		}
	}
	for queue, want := range map[string]int{consts.QueueDeliveryHistory: 20, consts.QueueDeliveryRealtime: 1, consts.QueueDeliveryUrgentLive: 1, consts.QueueDeliveryUrgent: 0} {
		tasks, err := inspector.ListPendingTasks(queue)
		if want == 0 && errors.Is(err, asynq.ErrQueueNotFound) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != want {
			t.Fatalf("queue %s pending=%d want=%d", queue, len(tasks), want)
		}
	}
}

func TestReplayDeliveryCanMovePastStaleUniqueTask(t *testing.T) {
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	collector := &sCollector{queueClient: client}
	ctx := context.Background()

	// Simulate a delivery already parked in the history queue under Asynq's
	// unique marker, then replay the same delivery at urgent priority.
	if err := collector.enqueueDeliveryTask(ctx, 991, sysin.EventPriorityNormal); err != nil {
		t.Fatal(err)
	}
	if err := collector.enqueueDeliveryTaskWithOptions(ctx, 991, sysin.EventPriorityUrgent, false); err != nil {
		t.Fatal(err)
	}
	history, err := inspector.ListPendingTasks(consts.QueueDeliveryHistory)
	if err != nil || len(history) != 1 {
		t.Fatalf("stale history task count=%d err=%v", len(history), err)
	}
	urgent, err := inspector.ListPendingTasks(consts.QueueDeliveryUrgentLive)
	if err != nil || len(urgent) != 1 {
		t.Fatalf("replay urgent task count=%d err=%v", len(urgent), err)
	}
}

func TestDeliveryRecoveryQuarantinesBadEventAndContinuesLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.Background()
	_ = g.DB().GetCore().ClearTableFieldsAll(ctx)
	defer g.DB().GetCore().ClearTableFieldsAll(ctx)
	redis := miniredis.RunT(t)
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + redis.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	cache.SetAdapter(ctx)
	defer func() {
		g.Cfg().SetAdapter(previousConfig)
		cache.SetAdapter(ctx)
	}()
	if err := markCollectorRecoveryScope(ctx, collectorRecoveryScope{TenantID: 1}); err != nil {
		t.Fatal(err)
	}
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_tg_collector_event(id bigint,source_type text,raw_update jsonb,created_at timestamp,received_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_tg_collector_delivery(id bigint PRIMARY KEY,event_id bigint,status text,priority int,updated_at timestamp,next_run_at timestamp,attempt_count int,error_message text,tenant_id bigint DEFAULT 1,account_id bigint DEFAULT 0,source_id bigint DEFAULT 0,scheduler_pool text DEFAULT 'realtime',created_at timestamp DEFAULT (CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai') - interval '1 hour') ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_event VALUES(1,'account','"malformed"',now(),now()),(2,'account','{"history":false}',now(),now())`,
			`INSERT INTO hg_tg_collector_delivery(id,event_id,status,priority,updated_at,next_run_at,attempt_count,error_message) VALUES(1,1,'pending',100,now()-interval '1 hour',NULL,0,''),(2,2,'pending',50,now()-interval '1 hour',NULL,0,'')`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		collector := &sCollector{queueClient: client}
		if err := collector.enqueueDueDeliveryTasks(ctx, 10); err != nil {
			return err
		}
		row, err := tx.Ctx(ctx).GetOne(`SELECT status FROM hg_tg_collector_delivery WHERE id=1`)
		if err != nil {
			return err
		}
		if row["status"].String() != sysin.DeliveryStatusQuarantined {
			t.Fatal("malformed event was not quarantined")
		}
		tasks, err := inspector.ListPendingTasks(consts.QueueDeliveryRealtime)
		if err != nil {
			return err
		}
		if len(tasks) != 1 {
			t.Fatalf("valid task after malformed event was not recovered: %d", len(tasks))
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := collector.persistedDeliveryPriority(cancelled, 2); err == nil || errors.Is(err, errDeliveryInvalidEvent) {
			t.Fatal("transient DB failure must remain retryable")
		}
		tx.Ctx(ctx) // Child operation contexts must not leak into fixture commit.
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectorRuntimeStopsWithoutSignal(t *testing.T) {
	redis := miniredis.RunT(t)
	previous := g.Cfg().GetAdapter()
	defer g.Cfg().SetAdapter(previous)
	adapter, err := gcfg.NewAdapterContent("telegramCollector:\n  enabled: true\nredis:\n  default:\n    address: '" + redis.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	collector := &sCollector{}
	for i := 0; i < 2; i++ {
		collector.StartRuntime(context.Background())
		collector.StartDeliveryRuntime(context.Background())
		stopped := make(chan struct{})
		go func() { collector.StopRuntime(); close(stopped) }()
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			t.Fatal("runtime shutdown still waits for an OS signal")
		}
	}
}

func TestCollectorIngressHistoryUsesDedicatedQueue(t *testing.T) {

	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	previousDB := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", previousDB)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	redis := miniredis.RunT(t)
	previousConfig := g.Cfg().GetAdapter()
	defer g.Cfg().SetAdapter(previousConfig)
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + redis.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_tg_collector_event(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_event SELECT id,1,0,0 FROM generate_series(1,21) id`); err != nil {
			return err
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(ctx) }()

		opt := asynq.RedisClientOpt{Addr: redis.Addr()}
		client := asynq.NewClient(opt)
		defer client.Close()
		inspector := asynq.NewInspector(opt)
		defer inspector.Close()
		collector := &sCollector{queueClient: client}
		for id := int64(1); id <= 20; id++ {
			if err := collector.enqueueEventTask(ctx, id, sysin.EventPriorityNormal); err != nil {
				t.Fatal(err)
			}
		}
		if err := collector.enqueueEventTask(ctx, 21, sysin.EventPriorityRealtime); err != nil {
			t.Fatal(err)
		}
		for queue, want := range map[string]int{consts.QueueCollectHistory: 20, consts.QueueCollectRealtime: 1} {
			tasks, err := inspector.ListPendingTasks(queue)
			if err != nil {
				t.Fatal(err)
			}
			if len(tasks) != want {
				t.Fatalf("%s jobs=%d want=%d", queue, len(tasks), want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

}

func TestDeliverySchedulerPoolEnforcesSourceAndAccountQuotas(t *testing.T) {
	accountCounts := map[int64]int{615: 1}
	sourceCounts := map[int64]int{351: 1}

	if !historyDeliveryWithinQuota(615, 351, accountCounts, sourceCounts, 2, 2) {
		t.Fatal("source/account scheduler pool should admit work below both quotas")
	}
	if historyDeliveryWithinQuota(615, 351, accountCounts, sourceCounts, 1, 2) {
		t.Fatal("account scheduler quota must reject work at the account limit")
	}
	if historyDeliveryWithinQuota(615, 351, accountCounts, sourceCounts, 2, 1) {
		t.Fatal("source scheduler quota must reject work at the source limit")
	}
	if !historyDeliveryWithinQuota(0, 351, accountCounts, sourceCounts, 2, 2) {
		t.Fatal("bot source should be admitted below the source quota when account_id is zero")
	}
	if historyDeliveryWithinQuota(0, 351, accountCounts, sourceCounts, 2, 1) {
		t.Fatal("bot source must still obey the source quota when account_id is zero")
	}
	if !historyDeliveryWithinQuota(0, 352, accountCounts, sourceCounts, 2, 2) {
		t.Fatal("a different source must have an independent scheduler quota")
	}
}

func TestDeliverySourceShardIsStableAndAccountScoped(t *testing.T) {
	first := consts.DeliveryRealtimeShardQueue(615)
	retry := consts.DeliveryRealtimeShardQueue(615)
	other := consts.DeliveryRealtimeShardQueue(616)

	if first != retry {
		t.Fatalf("same account must remain on the same realtime shard: %q != %q", first, retry)
	}
	if first == other {
		t.Fatalf("different accounts should not use this test's selected adjacent shards: %q", first)
	}
	if !strings.HasPrefix(first, consts.QueueDeliveryRealtimeShardPrefix) {
		t.Fatalf("realtime shard has unexpected prefix: %q", first)
	}

	history := consts.DeliveryHistoryShardQueue(615)
	urgent := consts.DeliveryUrgentShardQueue(615)
	if history == first || urgent == first || history == urgent {
		t.Fatalf("delivery classes must use distinct shard namespaces: realtime=%q history=%q urgent=%q", first, history, urgent)
	}
}

func TestDeliveryHistoryAndRealtimeQueueMapsAreDisjoint(t *testing.T) {
	history := deliveryHistoryQueues()
	realtime := deliveryRealtimeQueues()

	if len(history) == 0 || len(realtime) == 0 {
		t.Fatal("delivery queue maps must not be empty")
	}
	for queue := range history {
		if _, ok := realtime[queue]; ok {
			t.Fatalf("history and realtime workers must not share queue %q", queue)
		}
	}
	if _, ok := history[consts.QueueDeliveryUrgentLive]; ok {
		t.Fatal("urgent live queue must not be consumed by the history worker")
	}
	if _, ok := realtime[consts.QueueDeliveryHistory]; ok {
		t.Fatal("history queue must not be consumed by the realtime worker")
	}
}

func TestHistorySchedulerExcludesRealtimePriorityFromHistoryPool(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	schedulerPath := filepath.Join(filepath.Dir(currentFile), "history_scheduler.go")
	content, err := os.ReadFile(schedulerPath)
	if err != nil {
		t.Fatalf("read history scheduler source: %v", err)
	}
	source := string(content)

	if !strings.Contains(source, `Where("scheduler_pool", historySchedulerPool)`) {
		t.Fatal("history scheduler must claim only the history scheduler pool")
	}
	if !strings.Contains(source, `Where(columns.Priority, "<", sysin.EventPriorityRealtime)`) {
		t.Fatal("history scheduler must exclude realtime and urgent priorities")
	}
	if historySchedulerPool != "history" {
		t.Fatalf("history scheduler pool = %q, want history", historySchedulerPool)
	}
}

func TestDeliverySchedulerPoolClassifiesLegacyAndRealtimeQueues(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	queuePath := filepath.Join(filepath.Dir(currentFile), "delivery_queue.go")
	content, err := os.ReadFile(queuePath)
	if err != nil {
		t.Fatalf("read delivery queue source: %v", err)
	}
	source := string(content)

	if !strings.Contains(source, `schedulerPool == "history" && priority < sysin.EventPriorityRealtime`) {
		t.Fatal("legacy history scheduler_pool must remain history-only for normal priority")
	}
	if !strings.Contains(source, `Fields("account_id", "source_id", "scheduler_pool")`) {
		t.Fatal("delivery routing must read account_id, source_id and scheduler_pool from delivery")
	}
	if strings.Contains(source, `LeftJoin("hg_tg_collector_event e"`) {
		t.Fatal("delivery routing must not join collector events")
	}
	if !strings.Contains(source, `if priority >= sysin.EventPriorityRealtime`) {
		t.Fatal("realtime delivery must have an explicit priority classification")
	}
	if !strings.Contains(source, "consts.DeliveryLiveSourceQueue(sourceID)") {
		t.Fatal("source-scoped realtime delivery must use the live source queue")
	}
	if !strings.Contains(source, "consts.DeliveryHistorySourceQueue(sourceID)") {
		t.Fatal("source-scoped history delivery must use the history source queue")
	}
}

func TestDeliverySourceHistoryAndLiveShardNamespacesAreDisjoint(t *testing.T) {
	for _, sourceID := range []int64{1, 351, 615, 1024} {
		history := consts.DeliveryHistorySourceQueue(sourceID)
		live := consts.DeliveryLiveSourceQueue(sourceID)
		if history == live {
			t.Fatalf("source %d history and live queues must be distinct: %q", sourceID, history)
		}
		if !strings.HasPrefix(history, consts.QueueDeliveryHistorySourceShardPrefix) {
			t.Fatalf("source %d history queue has wrong namespace: %q", sourceID, history)
		}
		if !strings.HasPrefix(live, consts.QueueDeliveryLiveSourceShardPrefix) {
			t.Fatalf("source %d live queue has wrong namespace: %q", sourceID, live)
		}
	}
}

// Recovery scheduling and paging deliberately live in separate files. Assert
// each contract at its owner so unrelated scoped queries cannot mask a bypass.
func deliveryRecoverySource(t *testing.T, name string) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(content)
}

func TestDeliveryRecoveryUsesBoundedSchedulerPoolLanes(t *testing.T) {
	queue := deliveryRecoverySource(t, "delivery_queue.go")
	cursor := deliveryRecoverySource(t, "delivery_recovery_cursor.go")
	start := strings.Index(queue, "func (s *sCollector) enqueueDueDeliveryTasks(")
	if start < 0 {
		t.Fatal("recovery scheduler missing")
	}
	end := strings.Index(queue[start:], "\nfunc ")
	if end < 0 {
		t.Fatal("recovery scheduler boundary missing")
	}
	scheduler := queue[start : start+end]
	for _, required := range []string{
		"collectorRecoveryScopes(scopeCtx, collectorRecoveryDeliveryEnqueue, 8)",
		`collectorRecoveryBranchOrder([]string{"realtime", "history"})`,
		"min(laneQuota, remaining)", "s.recoverDeliveryCursorPage(laneCtx, scope, pool,",
	} {
		if !strings.Contains(scheduler, required) {
			t.Fatalf("scoped scheduler missing %s", required)
		}
	}
	start = strings.Index(cursor, "func deliveryRecoveryPage(")
	if start < 0 {
		t.Fatal("recovery page missing")
	}
	end = strings.Index(cursor[start:], "\nfunc ")
	if end < 0 {
		t.Fatal("recovery page boundary missing")
	}
	page := cursor[start : start+end]
	for _, required := range []string{
		"!scope.valid()", "Where(cols.TenantId, scope.TenantID)",
		`Where("account_id", scope.AccountID)`, `Where("source_id", scope.SourceID)`,
		`Where("scheduler_pool", pool)`, "min(limit, deliveryRecoveryPageSize)",
		".Limit(limit).All()", "cursor.AfterID <= 0", "WhereLTE(cols.CreatedAt, cursor.UntilTime)",
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("scoped cursor query missing %s", required)
		}
	}
	for _, source := range []string{queue, cursor} {
		if strings.Contains(source, "ROW_NUMBER() OVER") {
			t.Fatal("delivery recovery must not run global source ranking")
		}
	}
}

func TestDeliveryRecoveryDoesNotTreatDuplicateAsSuccessfulEnqueue(t *testing.T) {
	queue := deliveryRecoverySource(t, "delivery_queue.go")
	cursor := deliveryRecoverySource(t, "delivery_recovery_cursor.go")
	if !strings.Contains(cursor, "enqueueDeliveryTaskWithOptions(enqueueCtx, deliveryID, priority, false)") {
		t.Fatal("delivery recovery must enqueue without Asynq Unique")
	}
	if !strings.Contains(queue, "if !unique {") || !strings.Contains(queue, "恢复Telegram采集交付时任务已存在，未确认本次入队") {
		t.Fatal("non-unique recovery enqueue must return duplicate conflicts as errors")
	}
	if !strings.Contains(queue, `recovery.enqueue_failed`) {
		t.Fatal("recovery enqueue failures must be emitted as structured logs")
	}
	if !strings.Contains(cursor, "enqueueObservation.Finish(collectorResultError, err)") {
		t.Fatal("recovery conflicts must be observed as failures")
	}
	if !strings.Contains(cursor, "resultErr = errors.Join(resultErr, err)") || !strings.Contains(cursor, "return cursor, resultErr") {
		t.Fatal("scan progress must not hide failed handoff errors")
	}
}
