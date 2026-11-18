package sys

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/utility/sourcequeue"
)

// Temporary tables and a child process isolate both SQL and GoFrame's globals.
func TestBotSourceReplayLoadedLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT") != "55489" {
		t.Skip("requires dedicated loopback PostgreSQL 55489")
	}
	if !collectorBatchPostgresChild(t) {
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55489", User: "postgres", Name: "postgres"}})
	r := miniredis.RunT(t)
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + r.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	cache.SetAdapter(context.Background())
	schema, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	ddl := string(schema)
	ddl = ddl[strings.Index(ddl, `CREATE TABLE IF NOT EXISTS "hg_tg_collector_event"`):]
	ddl = ddl[:strings.Index(ddl, ");")+2]
	ddl = strings.Replace(ddl, "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1)
	err = g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(ddl); err != nil {
			return err
		}
		// BEFORE INSERT counts even ON CONFLICT attempts, proving the shortcut
		// bypasses persistEvent rather than merely relying on its uniqueness key.
		for _, sql := range []string{
			`CREATE TEMP TABLE replay_insert_attempts(id bigint)`,
			`CREATE FUNCTION pg_temp.replay_count_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO replay_insert_attempts VALUES(NEW.id); RETURN NEW; END $$`,
			`CREATE TRIGGER replay_insert BEFORE INSERT ON hg_tg_collector_event FOR EACH ROW EXECUTE FUNCTION pg_temp.replay_count_insert()`,
			`CREATE TEMP TABLE hg_tg_collector_delivery(id bigserial PRIMARY KEY,tenant_id bigint,event_id bigint,delivery_key text,status text,source_id bigint,account_id bigint DEFAULT 0,created_at timestamp,priority integer,scheduler_pool text,attempt_count integer,next_run_at timestamp,lease_owner text,lease_until timestamp,error_message text,updated_at timestamp,CONSTRAINT replay_delivery_identity UNIQUE(tenant_id,delivery_key))`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		s := &sCollector{}
		insert := func(update int64, status, text string) int64 {
			t.Helper()
			raw := fmt.Sprintf(`{"update_id":%d,"channel_post":{"message_id":%d,"chat":{"id":-100123456,"type":"channel"},"text":%q}}`, update, update, text)
			id, err := tx.GetValue(`INSERT INTO hg_tg_collector_event(tenant_id,source_id,source_type,bot_key,chat_id,message_id,update_id,event_key,raw_update,status,priority,received_at) VALUES(78,351,'bot','test-bot','-100123456',$1,$2,$3,CAST($4 AS jsonb),$5,100,'2026-01-01') RETURNING id`, update, update, updateEventID("test-bot", 78, 351, update, []byte(raw)), raw, status)
			if err != nil {
				t.Fatal(err)
			}
			return id.Int64()
		}
		attempts := func() int {
			t.Helper()
			n, err := tx.GetValue(`SELECT count(*) FROM replay_insert_attempts`)
			if err != nil {
				t.Fatal(err)
			}
			return n.Int()
		}
		received := insert(1, sysin.EventStatusReceived, "普通资料")
		processing := insert(2, sysin.EventStatusProcessing, "普通资料")
		ready := insert(3, sysin.EventStatusReady, "普通资料")
		notice := insert(4, sysin.EventStatusIgnored, "✅提交成功")
		dead := insert(5, sysin.EventStatusDead, "普通资料")
		leased := insert(6, sysin.EventStatusDead, "普通资料")
		if _, err := tx.Exec(`UPDATE hg_tg_collector_event SET lease_until='2099-01-01',lease_owner='other',attempt_count=3 WHERE id=?`, leased); err != nil {
			return err
		}
		readyKey := collectorDeliveryKey(&entity.TgCollectorEvent{TenantId: 78, SourceId: 351, SourceType: sysin.SourceTypeBot, EventKey: updateEventID("test-bot", 78, 351, 3, nil)}, nil)
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery(tenant_id,event_id,delivery_key,status,source_id) VALUES(78,?,?,'completed',351)`, ready, readyKey); err != nil {
			return err
		}
		before := attempts()
		result, err := s.ReplayBotUpdatesForSource(ctx, 78, 351, []int64{received, processing, ready, notice, dead, leased, received})
		if err != nil || result.Queued != 2 || result.Skipped != 4 || len(result.Failed) != 0 || attempts() != before {
			t.Fatalf("same-source result=%+v err=%v insert attempts=%d", result, err, attempts()-before)
		}
		q, err := sourcequeue.New(collectorEventSourceNamespace)
		if err != nil {
			return err
		}
		for _, id := range []int64{received, dead} {
			ok, err := q.HasTask(ctx, sourcequeue.Scope{TenantID: 78, SourceID: 351}, sourcequeue.History, strconv.FormatInt(id, 10))
			if err != nil || !ok {
				t.Fatalf("history task %d missing: %v", id, err)
			}
			live, err := q.HasTask(ctx, sourcequeue.Scope{TenantID: 78, SourceID: 351}, sourcequeue.Realtime, strconv.FormatInt(id, 10))
			if err != nil || live {
				t.Fatalf("history replay leaked into realtime: %d %v", id, err)
			}
		}
		scopes, err := collectorRecoveryScopes(ctx, collectorRecoveryEventEnqueue, 50)
		if err != nil || len(scopes) != 1 || scopes[0] != (collectorRecoveryScope{78, 0, 351}) {
			t.Fatalf("same-source recovery scope missing: %v %v", scopes, err)
		}
		rows, err := tx.GetAll(`SELECT status,lease_owner,attempt_count FROM hg_tg_collector_event ORDER BY id`)
		if err != nil {
			return err
		}
		if rows[1]["status"].String() != "processing" || rows[2]["status"].String() != "ready" || rows[3]["status"].String() != "ignored" || rows[5]["lease_owner"].String() != "other" || rows[5]["attempt_count"].Int() != 3 {
			t.Fatalf("protected state changed: %v", rows)
		}
		// A batch snapshot cannot reset an event another worker has since claimed.
		_, stale, err := s.loadBotUpdateReplayCandidates(ctx, 78, []int64{leased})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE hg_tg_collector_event SET status='processing',lease_until=NULL WHERE id=?`, leased); err != nil {
			return err
		}
		if queued, err := s.enqueueReplayLoadedEvent(ctx, stale[leased]); err != nil || queued {
			t.Fatalf("stale snapshot reset a processing event: queued=%v err=%v", queued, err)
		}
		// Cross-source replay retains persistEvent, creates a scoped identity,
		// and repeated replay does not create a second target event.
		for range 2 {
			result, err = s.ReplayBotUpdatesForSource(ctx, 78, 352, []int64{received})
			if err != nil || result.Queued != 1 || len(result.Failed) != 0 {
				t.Fatalf("cross-source result=%+v err=%v", result, err)
			}
		}
		if attempts() != before+2 {
			t.Fatal("cross-source replay did not preserve upsert path")
		}
		target, err := tx.GetAll(`SELECT id,event_key,source_id,received_at,priority FROM hg_tg_collector_event WHERE source_id=352`)
		if err != nil {
			return err
		}
		if len(target) != 1 || target[0]["event_key"].String() != updateEventID("test-bot", 78, 352, 1, nil) || target[0]["received_at"].GTime().Format("Y-m-d") != "2026-01-01" {
			t.Fatalf("cross-source identity or timestamp changed: %v", target)
		}
		targetID := target[0]["id"].Int64()
		_, targetEvents, err := s.loadBotUpdateReplayCandidates(ctx, 78, []int64{targetID})
		if err != nil {
			return err
		}
		targetEvent := targetEvents[targetID]
		targetKey := collectorDeliveryKey(targetEvent, []byte(targetEvent.RawUpdate.String()))
		if _, err := tx.Exec(`UPDATE hg_tg_collector_event SET status='ready' WHERE id=?`, targetID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery(tenant_id,event_id,delivery_key,status,source_id) VALUES(78,?,?,'completed',352)`, targetID, targetKey); err != nil {
			return err
		}
		result, err = s.ReplayBotUpdatesForSource(ctx, 78, 352, []int64{received})
		if err != nil || result.Queued != 0 || result.Skipped != 1 || len(result.Failed) != 0 {
			t.Fatalf("cross-source ready replay used wrong delivery identity: %+v %v", result, err)
		}
		candidates, loaded, err := s.loadBotUpdateReplayCandidates(ctx, 78, []int64{received})
		if err != nil {
			return err
		}
		for name, mutate := range map[string]func(*entity.TgCollectorEvent){
			"identity": func(row *entity.TgCollectorEvent) { row.EventKey = "legacy" },
			"update":   func(row *entity.TgCollectorEvent) { row.UpdateId++ },
			"chat":     func(row *entity.TgCollectorEvent) { row.ChatId = "-100999" },
			"message":  func(row *entity.TgCollectorEvent) { row.MessageId++ },
			"account":  func(row *entity.TgCollectorEvent) { row.AccountId = 99 },
		} {
			row := *loaded[received]
			mutate(&row)
			before := attempts()
			queued, err := s.replayLoadedBotUpdateForSource(ctx, candidates[0], &row, 351)
			if err != nil || !queued || attempts() != before+1 {
				t.Fatalf("%s mismatch bypassed persistence: queued=%v err=%v", name, queued, err)
			}
		}
		for name, mutate := range map[string]func(*entity.TgCollectorEvent){
			"id":     func(row *entity.TgCollectorEvent) { row.Id++ },
			"tenant": func(row *entity.TgCollectorEvent) { row.TenantId++ },
			"source": func(row *entity.TgCollectorEvent) { row.SourceId++ },
			"bot":    func(row *entity.TgCollectorEvent) { row.BotKey = "other-bot" },
			"type":   func(row *entity.TgCollectorEvent) { row.SourceType = sysin.SourceTypeAccount },
		} {
			row := *loaded[received]
			mutate(&row)
			before := attempts()
			queued, err := s.replayLoadedBotUpdateForSource(ctx, candidates[0], &row, 351)
			if err == nil || queued || attempts() != before {
				t.Fatalf("%s mismatch accepted: queued=%v err=%v", name, queued, err)
			}
		}
		// A legacy key must not qualify for the same-source shortcut.
		if _, err := tx.Exec(`UPDATE hg_tg_collector_event SET event_key='legacy' WHERE id=?`, processing); err != nil {
			return err
		}
		before = attempts()
		result, err = s.ReplayBotUpdatesForSource(ctx, 78, 351, []int64{processing})
		if err != nil || result.Queued != 1 || len(result.Failed) != 0 || attempts() != before+1 {
			t.Fatalf("legacy identity result=%+v err=%v", result, err)
		}
		// The only delivery lookup index is tenant+delivery_key, exactly as in
		// production before the separately managed online event_id index.
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery(tenant_id,event_id,delivery_key,status,source_id) SELECT 99,n,'padding:'||n,'completed',999 FROM generate_series(1,10000) n`); err != nil {
			return err
		}
		if _, err := tx.Exec(`ANALYZE hg_tg_collector_delivery`); err != nil {
			return err
		}
		for i, tc := range []struct {
			name        string
			key         string
			collision   bool
			wrongSource bool
			staleLegacy bool
			missing     bool
		}{
			{name: "canonical", key: "canonical"},
			{name: "repair", key: "repair", collision: true},
			{name: "legacy", key: "legacy"},
			{name: "wrong source", key: "canonical", wrongSource: true},
			{name: "reused legacy id", key: "legacy", staleLegacy: true},
			{name: "missing", missing: true},
		} {
			id := insert(int64(100+i), sysin.EventStatusReady, "索引恢复样本")
			if _, err := tx.Exec(`UPDATE hg_tg_collector_event SET created_at='2026-01-01' WHERE id=?`, id); err != nil {
				return err
			}
			_, loaded, err := s.loadBotUpdateReplayCandidates(ctx, 78, []int64{id})
			if err != nil {
				return err
			}
			event := loaded[id]
			base := collectorDeliveryKey(event, []byte(event.RawUpdate.String()))
			key := base
			if tc.key == "repair" {
				key = collectorDeliveryCollisionKey(base, id)
			} else if tc.key == "legacy" {
				key = fmt.Sprintf("event:%d", id)
			}
			if tc.collision {
				if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery(tenant_id,event_id,delivery_key,status,source_id) VALUES(78,999999,?,'completed',351)`, base); err != nil {
					return err
				}
			}
			if !tc.missing {
				source := int64(351)
				created := "2026-01-02"
				if tc.wrongSource {
					source = 999
				}
				if tc.staleLegacy {
					created = "2025-01-01"
				}
				if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery(tenant_id,event_id,delivery_key,status,source_id,created_at) VALUES(78,?,?,'completed',?,?)`, id, key, source, created); err != nil {
					return err
				}
			}
			var queued bool
			sqls, err := gdb.CatchSQL(ctx, func(ctx context.Context) error {
				var err error
				queued, err = s.enqueueReplayLoadedEvent(ctx, event)
				return err
			})
			if err != nil && !tc.wrongSource {
				return err
			}
			if tc.wrongSource && err == nil {
				t.Fatal("corrupt canonical source identity accepted")
			}
			wantRecovery := tc.staleLegacy || tc.missing
			if queued != wantRecovery {
				t.Fatalf("%s queued=%v want recovery=%v", tc.name, queued, wantRecovery)
			}
			selects := 0
			for _, sql := range sqls {
				if !strings.HasPrefix(strings.ToUpper(sql), "SELECT") || !strings.Contains(sql, "hg_tg_collector_delivery") {
					continue
				}
				selects++
				if !strings.Contains(sql, "tenant_id") || !strings.Contains(sql, "delivery_key") {
					t.Fatalf("unindexed replay lookup: %s", sql)
				}
				plan, err := tx.GetAll("EXPLAIN " + sql)
				if err != nil {
					return err
				}
				if !strings.Contains(fmt.Sprint(plan), "replay_delivery_identity") || strings.Contains(fmt.Sprint(plan), "Seq Scan") {
					t.Fatalf("%s did not use unique key: %v", tc.name, plan)
				}
			}
			if selects < 1 || selects > 3 {
				t.Fatalf("%s unbounded probes=%d", tc.name, selects)
			}
			t.Logf("%s: %d delivery probes, all use replay_delivery_identity", tc.name, selects)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
