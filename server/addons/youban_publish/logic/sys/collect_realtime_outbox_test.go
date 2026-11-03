package sys

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
)

const createRealtimeOutboxTestSQL = `CREATE TEMP TABLE hg_youban_publish_collect_process_outbox(id bigserial PRIMARY KEY,event_id bigint UNIQUE,tenant_id bigint,account_id bigint,source_id bigint,source_chat_id text,status text,available_at timestamp,lease_owner text DEFAULT '',lease_expires_at timestamp,attempt_count int DEFAULT 0,last_error text,delivered_at timestamp,created_at timestamp,updated_at timestamp,history boolean) ON COMMIT DROP`

func TestCollectorHistoryReachesBusinessRouting(t *testing.T) {
	for _, history := range []bool{false, true} {
		msg := collectorDeliveryMessage(&collectorin.CollectorDelivery{History: history}, 1, 2, 3, "account")
		if msg.History != history {
			t.Fatal("delivery classification was lost")
		}
		want := tgQueueNameCollectProcessRealtimeV2
		if history {
			want = tgQueueNameCollectProcessRecoveryV2
		}
		if got := collectProcessQueueName(collectProcessQueuePayload{EventId: 1, OutboxOwner: "realtime-owner", History: msg.History}); got != want {
			t.Fatalf("queue=%s want=%s", got, want)
		}
	}
}

func TestRealtimeHistoryAndExpiredOutboxFairnessLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	_ = g.DB() // initialize the lazy default group before saving/restoring it
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
	s := &sSysPublish{tgQueueClient: client}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			createRealtimeOutboxTestSQL,
			`CREATE TEMP TABLE hg_youban_publish_collect_event(id bigint,received_at timestamp,created_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_process_outbox(event_id,tenant_id,account_id,source_id,status,available_at,created_at,updated_at,history) SELECT i,1,2,3,'pending',LOCALTIMESTAMP-INTERVAL '1 second',LOCALTIMESTAMP,LOCALTIMESTAMP,FALSE FROM generate_series(1,100) i`,
			`INSERT INTO hg_youban_publish_collect_process_outbox(event_id,tenant_id,account_id,source_id,status,available_at,created_at,updated_at,history) SELECT i,1,2,3,'pending',LOCALTIMESTAMP-INTERVAL '1 second',LOCALTIMESTAMP,LOCALTIMESTAMP,TRUE FROM generate_series(101,200) i`,
			`INSERT INTO hg_youban_publish_collect_process_outbox(event_id,tenant_id,account_id,source_id,status,available_at,created_at,updated_at,lease_expires_at,history) SELECT i,1,2,3,'processing',LOCALTIMESTAMP-INTERVAL '20 minutes',LOCALTIMESTAMP-INTERVAL '20 minutes',LOCALTIMESTAMP-INTERVAL '20 minutes',LOCALTIMESTAMP-INTERVAL '10 minutes',FALSE FROM generate_series(201,205) i`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		// Saturated fresh ingress must not consume the history or abandoned lease slots.
		if err := s.dispatchDueCollectProcessOutbox(ctx, 10); err != nil {
			return err
		}
		for queue, want := range map[string]int{tgQueueNameCollectProcessRealtimeV2: 6, tgQueueNameCollectProcessRecoveryV2: 4} {
			jobs, err := inspector.ListPendingTasks(queue)
			if err != nil {
				return err
			}
			if len(jobs) != want {
				t.Fatalf("%s jobs=%d want=%d", queue, len(jobs), want)
			}
		}
		n, err := tx.GetOne(`SELECT COUNT(*) n FROM hg_youban_publish_collect_process_outbox WHERE event_id>=201 AND lease_expires_at>LOCALTIMESTAMP`)
		if err != nil {
			return err
		}
		if n["n"].Int() != 2 {
			t.Fatal("expired old rows were excluded")
		}
		// Explicit history is persisted, including repeated/recovery submissions.
		p := collectProcessQueuePayload{EventId: 500, TenantId: 1, AccountId: 2, SourceId: 3, History: true}
		if err := ensureCollectProcessOutbox(ctx, p, 0); err != nil {
			return err
		}
		row, err := tx.GetOne(`SELECT history FROM hg_youban_publish_collect_process_outbox WHERE event_id=500`)
		if err != nil {
			return err
		}
		if !row["history"].Bool() {
			t.Fatal("history lost at persistence")
		}
		// Legacy rows require original event-time classification, not fresh Outbox time.
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_process_outbox SET status='delivered' WHERE event_id<500`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_event VALUES(501,LOCALTIMESTAMP-INTERVAL '1 month',LOCALTIMESTAMP); INSERT INTO hg_youban_publish_collect_process_outbox(event_id,tenant_id,account_id,source_id,status,available_at,created_at,updated_at) VALUES(501,1,2,3,'pending',LOCALTIMESTAMP-INTERVAL '1 second',LOCALTIMESTAMP,LOCALTIMESTAMP)`); err != nil {
			return err
		}
		if err := s.dispatchDueCollectProcessOutbox(ctx, 10); err != nil {
			return err
		}
		row, err = tx.GetOne(`SELECT lease_owner FROM hg_youban_publish_collect_process_outbox WHERE event_id=501`)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(row["lease_owner"].String(), "recovery-") {
			t.Fatal("legacy history leaked into realtime")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectMissingHistoryColumnIsNotIgnored(t *testing.T) {
	if isMissingCollectProcessOutboxError(assertionError(`column "history" of relation "hg_youban_publish_collect_process_outbox" does not exist`)) {
		t.Fatal("required routing migration was silently bypassed")
	}
}

func TestDelayedBotDeliveryUsesHistoryBusinessPool(t *testing.T) {
	msg := collectorDeliveryMessage(&collectorin.CollectorDelivery{ReceivedAt: time.Now().Add(-time.Hour)}, 1, 2, 3, "bot")
	if !msg.History {
		t.Fatal("old Bot backlog polluted fresh business pool")
	}
}

// Consume deliveries actually reconstructed/claimed by the collector replay,
// then exercise the real business adapter and durable schedule persistence.
func TestProductionDeliveryBusinessHandoffLocalPostgres(t *testing.T) {
	path, port := os.Getenv("YOUBAN_DELIVERY_CAPTURE_FILE"), os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if path == "" || port == "" {
		t.Skip("requires private collector capture and disposable PostgreSQL")
	}
	_ = g.DB()
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.Background()
	_ = g.DB().GetCore().ClearTableFieldsAll(ctx)
	defer g.DB().GetCore().ClearTableFieldsAll(ctx)
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("open private replay capture failed")
	}
	defer file.Close()
	count, history, live := 0, 0, 0
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(createRealtimeOutboxTestSQL); err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 65536), 8*1024*1024)
		for scanner.Scan() {
			var delivery collectorin.CollectorDelivery
			if json.Unmarshal(scanner.Bytes(), &delivery) != nil {
				t.Fatal("invalid private capture")
			}
			msg := collectorDeliveryMessage(&delivery, delivery.TenantID, delivery.AccountID, delivery.SourceID, delivery.SourceType)
			// Bot deliveries do not carry the business account ID; schedule persistence
			// is tested here with a fixed isolated owner, never by connecting production.
			p := collectProcessQueuePayload{EventId: delivery.EventID, TenantId: delivery.TenantID, AccountId: 1, SourceId: delivery.SourceID, History: msg.History}
			if err := ensureCollectProcessOutbox(ctx, p, 0); err != nil {
				return err
			}
			if err := ensureCollectProcessOutbox(ctx, p, 0); err != nil {
				return err
			}
			if msg.History {
				history++
			} else {
				live++
			}
			count++
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		row, err := tx.GetOne(`SELECT COUNT(*) n,COUNT(*) FILTER (WHERE history=TRUE) historical FROM hg_youban_publish_collect_process_outbox`)
		if err != nil {
			return err
		}
		if row["n"].Int() != count || row["historical"].Int() != history {
			t.Fatal("lost classification or duplicate durable handoff")
		}
		return nil
	})
	if err != nil {
		t.Fatal("private replay business handoff failed")
	}
	if count < 1000 || history == 0 || live == 0 {
		t.Fatal("capture lacks realtime/history diversity")
	}
	t.Logf("actual collector to business durable handoff rows=%d realtime=%d history_or_delayed=%d duplicates=0", count, live, history)
}

// Full production fixture replay: validates the real recovery dispatcher under
// online-shaped source/account distribution, not synthetic equal-sized rows.
func TestProductionFixtureRecoveryFairnessLocalPostgres(t *testing.T) {
	fixture := os.Getenv("YOUBAN_DELIVERY_CAPTURE_FILE")
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if fixture == "" || port == "" {
		t.Skip("requires private online capture and disposable loopback PostgreSQL")
	}
	_ = g.DB()
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	s := &sSysPublish{tgQueueClient: client}
	file, err := os.Open(fixture)
	if err != nil {
		t.Fatal("open online capture", err)
	}
	defer file.Close()
	ctx := context.Background()
	var rows, history, realtime int
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(createRealtimeOutboxTestSQL); err != nil {
			return err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 65536), 8*1024*1024)
		for scanner.Scan() {
			var delivery collectorin.CollectorDelivery
			if err := json.Unmarshal(scanner.Bytes(), &delivery); err != nil {
				return err
			}
			msg := collectorDeliveryMessage(&delivery, delivery.TenantID, delivery.AccountID, delivery.SourceID, delivery.SourceType)
			// Keep source/account distribution from production; account id 1 is
			// only the isolated test owner required by the queue payload contract.
			payload := collectProcessQueuePayload{EventId: delivery.EventID + int64(rows)*1000000, TenantId: delivery.TenantID, AccountId: 1, SourceId: delivery.SourceID, History: msg.History}
			if err := ensureCollectProcessOutbox(ctx, payload, 0); err != nil {
				return err
			}
			rows++
			if msg.History {
				history++
			} else {
				realtime++
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		if rows < 4000 || history == 0 || realtime == 0 {
			t.Fatalf("online fixture diversity missing rows=%d realtime=%d history=%d", rows, realtime, history)
		}
		if err := s.dispatchDueCollectProcessOutbox(ctx, 200); err != nil {
			return err
		}
		liveTasks, err := inspector.ListPendingTasks(tgQueueNameCollectProcessRealtimeV2)
		if err != nil {
			return err
		}
		recoveryTasks, err := inspector.ListPendingTasks(tgQueueNameCollectProcessRecoveryV2)
		if err != nil {
			return err
		}
		if len(liveTasks) == 0 || len(recoveryTasks) == 0 {
			t.Fatalf("dispatcher starved a lane live=%d recovery=%d", len(liveTasks), len(recoveryTasks))
		}
		if len(liveTasks)+len(recoveryTasks) > 200 {
			t.Fatalf("dispatcher exceeded batch=%d", len(liveTasks)+len(recoveryTasks))
		}
		// Both lanes must receive work; the exact count may be lower when
		// source events coalesce into an existing durable task identity.
		metric, err := tx.GetAll(`SELECT source_id,MIN(created_at) oldest,COUNT(*) pending FROM hg_youban_publish_collect_process_outbox WHERE status='pending' GROUP BY source_id ORDER BY oldest LIMIT 100`)
		if err != nil {
			return err
		}
		if len(metric) == 0 {
			t.Fatal("oldest pending source metric missing")
		}
		t.Logf("online fixture recovery fairness rows=%d realtime=%d history=%d claimed_live=%d claimed_recovery=%d oldest_source_metrics=%d", rows, realtime, history, len(liveTasks), len(recoveryTasks), len(metric))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
