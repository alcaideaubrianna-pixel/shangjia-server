package sys

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/microbatch"
)

func TestHistoryBatchPipelineLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT") != "55489" {
		t.Skip("requires dedicated loopback PostgreSQL 55489")
	}
	if !collectorBatchPostgresChild(t) {
		return
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55489", User: "postgres", Name: "postgres"}})
	ctx := context.Background()
	defer g.DB().Exec(ctx, `DROP TABLE IF EXISTS hg_tg_collector_delivery`)
	defer g.DB().Exec(ctx, `DROP TABLE IF EXISTS hg_tg_collector_event`)
	run := func() error {
		for _, ddl := range []string{
			`CREATE TABLE hg_tg_collector_event(id bigint PRIMARY KEY,tenant_id bigint,status text,lease_owner text,lease_until timestamp,attempt_count integer,updated_at timestamp,processed_at timestamp,next_run_at timestamp,error_message text,raw_update jsonb)`,
			`CREATE TABLE hg_tg_collector_delivery(id bigserial PRIMARY KEY,tenant_id bigint,event_id bigint,delivery_key text,status text,priority integer,source_id bigint,account_id bigint,scheduler_pool text,lease_owner text DEFAULT '',created_at timestamp,updated_at timestamp,UNIQUE(tenant_id,delivery_key))`,
			`INSERT INTO hg_tg_collector_event SELECT n,78,'received','',NULL,0,NULL,NULL,NULL,'','{}' FROM generate_series(1,20) n`,
		} {
			if _, err := g.DB().Exec(ctx, ddl); err != nil {
				return err
			}
		}
		var sqlCalls atomic.Int32
		s := &sCollector{runtimeCtx: ctx}
		s.historyEventBatch = microbatch.New(ctx, 20, 50*time.Millisecond, func(ctx context.Context, input []historyEventMutation) []microbatch.Result[entity.TgCollectorEvent] {
			s.historyBatchSQLMu.Lock()
			defer s.historyBatchSQLMu.Unlock()
			kinds := map[historyEventMutationKind]bool{}
			for _, in := range input {
				kinds[in.Kind] = true
			}
			sqlCalls.Add(int32(len(kinds)))
			return writeHistoryEventMutations(ctx, input)
		})
		s.historyDeliveryBatch = microbatch.New(ctx, 20, 50*time.Millisecond, func(ctx context.Context, input []sysin.CollectorDelivery) []microbatch.Result[historyDeliverySaved] {
			s.historyBatchSQLMu.Lock()
			defer s.historyBatchSQLMu.Unlock()
			sqlCalls.Add(1)
			return writeHistoryDeliveryBatch(ctx, input)
		})
		defer s.StopRuntime()
		var wg sync.WaitGroup
		start := make(chan struct{})
		for id := int64(1); id <= 20; id++ {
			wg.Add(1)
			go func(id int64) {
				defer wg.Done()
				<-start
				owner := fmt.Sprintf("owner-%d", id)
				row, err := s.historyEventBatch.Do(ctx, historyEventMutation{Kind: historyEventClaim, ID: id, Owner: owner})
				if err != nil || row.Id != id {
					t.Errorf("claim %d: row=%+v err=%v", id, row, err)
					return
				}
				saved, err := s.historyDeliveryBatch.Do(ctx, sysin.CollectorDelivery{TenantID: 78, SourceID: 351, EventID: id, DeliveryKey: fmt.Sprintf("batch-%d", id), History: true})
				if err != nil || saved.ID <= 0 || saved.EventID != id {
					t.Errorf("save %d: %+v %v", id, saved, err)
					return
				}
				ready, err := s.historyEventBatch.Do(ctx, historyEventMutation{Kind: historyEventReady, ID: id, TenantID: 78, Owner: owner, Attempt: row.AttemptCount})
				if err != nil || ready.Id != id {
					t.Errorf("ready %d: %+v %v", id, ready, err)
				}
			}(id)
		}
		close(start)
		wg.Wait()
		count, err := g.DB().GetValue(ctx, `SELECT count(*) FROM hg_tg_collector_event WHERE status='ready'`)
		if err != nil {
			return err
		}
		if count.Int() != 20 {
			t.Fatalf("ready=%d", count.Int())
		}
		t.Logf("20 events: selected persistence SQL statements=%d; previous per-event path=100", sqlCalls.Load())
		if sqlCalls.Load() >= 20 {
			t.Fatalf("coalescing did not reduce round trips: %d", sqlCalls.Load())
		}
		for _, ddl := range []string{`DROP TABLE hg_tg_collector_delivery`, `DROP TABLE hg_tg_collector_event`} {
			if _, err := g.DB().Exec(ctx, ddl); err != nil {
				return err
			}
		}
		return nil
	}
	err := run()
	if err != nil {
		t.Fatal(err)
	}
}
