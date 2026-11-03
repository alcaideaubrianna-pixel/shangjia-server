package sys

import (
	"context"
	"encoding/json"
	"errors"
	"os"
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
			`CREATE TEMP TABLE hg_tg_collector_delivery(id bigint,event_id bigint) ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_event(id,source_type,raw_update,created_at) VALUES(1,'account','{"history":true}',now()),(2,'account','{"history":false}',now()),(3,'account','{"receivedAt":"2020-01-01T00:00:00Z"}',now())`,
			`INSERT INTO hg_tg_collector_event VALUES(4,'account','{"receivedAt":"2026-11-02T22:12:00+08:00"}','2026-11-02 22:12:01','2026-11-02 22:12:00'),(5,'account','{"receivedAt":"2026-11-01T22:12:00+08:00"}','2026-11-02 22:12:01','2026-11-01 22:12:00')`,
			`INSERT INTO hg_tg_collector_delivery VALUES(11,1),(12,2),(13,3),(14,4),(15,5)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		for id, want := range map[int64]int{11: sysin.EventPriorityNormal, 12: sysin.EventPriorityRealtime, 13: sysin.EventPriorityNormal, 14: sysin.EventPriorityRealtime, 15: sysin.EventPriorityNormal} {
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
			if got := deliveryQueueName(deliveryPriority(&delivery)); got != tc.want {
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
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_tg_collector_event(id bigint,source_type text,raw_update jsonb,created_at timestamp,received_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_tg_collector_delivery(id bigint,event_id bigint,status text,priority int,updated_at timestamp,next_run_at timestamp,attempt_count int,error_message text) ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_event VALUES(1,'account','"malformed"',now(),now()),(2,'account','{"history":false}',now(),now())`,
			`INSERT INTO hg_tg_collector_delivery VALUES(1,1,'pending',100,now()-interval '1 hour',NULL,0,''),(2,2,'pending',50,now()-interval '1 hour',NULL,0,'')`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		collector := &sCollector{queueClient: client}
		if err := collector.enqueueDueDeliveryTasks(ctx, 10); err != nil {
			return err
		}
		row, err := tx.GetOne(`SELECT status FROM hg_tg_collector_delivery WHERE id=1`)
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
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	collector := &sCollector{queueClient: client}
	for id := int64(1); id <= 20; id++ {
		if err := collector.enqueueEventTask(context.Background(), id, sysin.EventPriorityNormal); err != nil {
			t.Fatal(err)
		}
	}
	if err := collector.enqueueEventTask(context.Background(), 21, sysin.EventPriorityRealtime); err != nil {
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
}
