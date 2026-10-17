package sys

import (
	"context"
	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"os"
	"testing"
	"time"
)

func TestCollectProcessProductionSnapshot(t *testing.T) {
	data, err := os.ReadFile("testdata/collect_process_production_20261017.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]interface{}
	if err = json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	terminal, active := 0, 0
	for _, row := range rows {
		record := gdb.Record{}
		for key, value := range row {
			record[key] = gvar.New(value)
		}
		got := collectProcessEventFinished(record)
		expected := row["status"] == "processed" || row["status"] == "ignored"
		if got != expected {
			t.Fatalf("production event %v finished=%v", row["id"], got)
		}
		if got {
			terminal++
		} else {
			active++
		}
	}
	if terminal < 8 || active < 3 {
		t.Fatalf("unrepresentative snapshot terminal=%d active=%d", terminal, active)
	}
	t.Logf("production snapshot: %d events, terminal=%d active=%d", len(rows), terminal, active)
}

func TestCollectProcessOutboxLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()

	service := &sSysPublish{tgQueueClient: client}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		_, err := tx.Exec(`CREATE TEMP TABLE hg_youban_publish_collect_process_outbox(id bigserial PRIMARY KEY,event_id bigint UNIQUE,tenant_id bigint,account_id bigint,source_id bigint,source_chat_id text,status text,available_at timestamp,lease_owner text DEFAULT '',lease_expires_at timestamp,attempt_count int DEFAULT 0,last_error text DEFAULT '',delivered_at timestamp,created_at timestamp,updated_at timestamp) ON COMMIT DROP`)
		if err != nil {
			return err
		}
		payload := collectProcessQueuePayload{EventId: 5343351, TenantId: 78, AccountId: 615, SourceId: 351}
		// Repeated collection notification only upserts one durable schedule.
		for i := 0; i < 20; i++ {
			if _, err = service.enqueueCollectProcessDeferred(ctx, payload, 0); err != nil {
				return err
			}
		}
		if err = service.dispatchDueCollectProcessOutbox(ctx, 100); err != nil {
			return err
		}
		first, err := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("event_id", payload.EventId).One()
		if err != nil {
			return err
		}
		owner := first["lease_owner"].String()
		if owner == "" {
			t.Fatal("missing owner")
		}
		for i := 0; i < 10; i++ {
			if _, err = tx.Exec("UPDATE " + collectProcessOutboxTable + " SET lease_expires_at=NOW()-INTERVAL '1 minute'"); err != nil {
				return err
			}
			if err = service.dispatchDueCollectProcessOutbox(ctx, 100); err != nil {
				return err
			}
		}
		pending, err := redis.List("asynq:{" + collectProcessQueueName(collectProcessQueuePayload{EventId: payload.EventId, OutboxOwner: owner}) + "}:pending")
		if err != nil {
			return err
		}
		if len(pending) != 1 {
			t.Fatalf("lease expiry duplicated task: pending=%d", len(pending))
		}
		// Recovery must not overwrite an already queued schedule or its owner.
		recovery := payload
		recovery.Recovery = true
		if err = ensureCollectProcessOutbox(ctx, recovery, 0); err != nil {
			return err
		}
		current, err := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("event_id", payload.EventId).One()
		if err != nil {
			return err
		}
		if current["lease_owner"].String() != owner {
			t.Fatal("recovery invalidated queued task")
		}
		stale := payload
		stale.OutboxOwner = "stale-owner"
		completeCollectProcessOutbox(ctx, stale)
		current, err = g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("event_id", payload.EventId).One()
		if err != nil {
			return err
		}
		if current["status"].String() != "processing" {
			t.Fatal("stale completion changed current schedule")
		}
		// A fresh notification supersedes the old generation; old completion cannot erase it.
		if err = ensureCollectProcessOutbox(ctx, payload, time.Minute); err != nil {
			return err
		}
		old := payload
		old.OutboxOwner = owner
		completeCollectProcessOutbox(ctx, old)
		current, err = g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("event_id", payload.EventId).One()
		if err != nil {
			return err
		}
		if current["status"].String() != "pending" {
			t.Fatal("old completion erased new schedule")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Run against a bounded, sanitized production dump in temporary local tables.
// No publish worker, Bot client, downloader or upload adapter is started.
func TestCollectProcessProductionDumpLocalPostgres(t *testing.T) {
	path := os.Getenv("YOUBAN_COLLECT_PROCESS_DUMP")
	if path == "" || os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires local production snapshot replay")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Events  []map[string]interface{} `json:"events"`
		Sources []map[string]interface{} `json:"sources"`
		Media   []map[string]interface{} `json:"media"`
		Outbox  []map[string]interface{} `json:"outbox"`
	}
	if err = json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	terminal, active := 0, 0
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_youban_publish_collect_event(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,source_chat_id text,source_message_id bigint,status text,material_role text,material_group_status text,material_parent_event_id bigint,error_message text,received_at timestamp,created_at timestamp,updated_at timestamp,processed_at timestamp) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,collect_enabled int,status int,channel_blacklisted int,deleted_at timestamp) ON COMMIT DROP`); err != nil {
			return err
		}
		raw, _ := json.Marshal(snapshot.Events)
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_event SELECT * FROM json_populate_recordset(NULL::hg_youban_publish_collect_event,?::json)`, string(raw)); err != nil {
			return err
		}
		raw, _ = json.Marshal(snapshot.Sources)
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_source SELECT * FROM json_populate_recordset(NULL::hg_youban_publish_collect_source,?::json)`, string(raw)); err != nil {
			return err
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		service := &sSysPublish{}
		for _, row := range snapshot.Events {
			record := gdb.Record{}
			for key, value := range row {
				record[key] = gvar.New(value)
			}
			payload := collectProcessQueuePayload{TenantId: record["tenant_id"].Int64(), AccountId: record["account_id"].Int64(), SourceId: record["source_id"].Int64(), SourceChatId: record["source_chat_id"].String(), EventId: record["id"].Int64()}
			if !collectProcessEventMatchesPayload(record, payload) {
				t.Fatal("snapshot owner mismatch")
			}
			if collectProcessEventFinished(record) {
				terminal++
				delay, pending, remove, err := service.processCollectSourceTask(ctx, payload)
				if err != nil || pending || !remove || delay != 0 {
					t.Fatalf("production terminal event %d failed idempotent consumption: delay=%s pending=%v remove=%v err=%v", payload.EventId, delay, pending, remove, err)
				}
			} else {
				active++
				// Actual SQL must admit pending/display records unless their settle gate or stale completion timestamp excludes them.
				if record["material_role"].String() != collectMaterialRoleVerify && record["processed_at"].IsNil() && record["material_group_status"].String() != collectMaterialGroupWaitingVerify {
					at := record["received_at"].GTime()
					if at == nil {
						at = record["created_at"].GTime()
					}
					rows, _, err := service.loadTargetCollectMaterialWindow(ctx, payload, record, at)
					if err != nil {
						return err
					}
					if len(rows) == 0 || rows[0]["id"].Int64() != payload.EventId {
						t.Fatalf("active production event %d excluded unexpectedly", payload.EventId)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if terminal == 0 || active == 0 {
		t.Fatal("snapshot must cover terminal and active events")
	}
	t.Logf("dump replay: events=%d terminal=%d active=%d media=%d outbox=%d sources=%d", len(snapshot.Events), terminal, active, len(snapshot.Media), len(snapshot.Outbox), len(snapshot.Sources))
}
