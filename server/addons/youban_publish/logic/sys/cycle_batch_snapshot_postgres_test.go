package sys

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
)

// This test deliberately opens a named localhost-only DB, with all fixtures in
// connection-local temporary tables. It never reads the application's default DB.
func TestCycleBatchSnapshotPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_BATCH_ROTATION_TEST_PG_PORT")
	if port == "" {
		t.Skip("set YOUBAN_BATCH_ROTATION_TEST_PG_PORT=55491 for disposable PG")
	}
	if port != "55491" {
		t.Fatal("isolated test requires localhost port 55491")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: port, User: "postgres", Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		exec := func(query string, args ...interface{}) error {
			_, err := tx.Exec(query, args...)
			return err
		}
		ddl := []string{
			`CREATE TEMP SEQUENCE hg_ybp_batch_order_seq START WITH 1000`,
			`CREATE TEMP TABLE hg_youban_publish_channel_profile
			(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,channel_id bigint,profile_id bigint,
			status text,batch_order bigint,batch_last_sent_job_id bigint DEFAULT 0,
			UNIQUE(channel_id,profile_id)) ON COMMIT DROP`,
			`CREATE INDEX idx_ybp_batch_rotation ON hg_youban_publish_channel_profile
			(tenant_id,channel_id,status,batch_order,profile_id) WHERE status='active' AND batch_order>0`,
			`CREATE TEMP TABLE hg_content_profile(id bigint PRIMARY KEY,status int,visibility text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint PRIMARY KEY,tenant_id bigint,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_channel
			(tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,is_manual int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_channel
			(id bigint PRIMARY KEY,tenant_id bigint,status int,publish_direction text,is_default_selected int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_cycle_batch_item
			(run_id bigint,ordinal bigint,tenant_id bigint,account_id bigint,channel_id bigint,profile_id bigint,
			batch_order bigint,job_id bigint DEFAULT 0,item_state text DEFAULT 'selected',error_message text DEFAULT '',
			PRIMARY KEY(run_id,ordinal),UNIQUE(run_id,profile_id)) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_channel VALUES(10,1,1,'up',1,NULL)`,
		}
		for _, query := range ddl {
			if err := exec(query); err != nil {
				return err
			}
		}
		seed := func(first, last int) error {
			for i := first; i <= last; i++ {
				// Descending profile IDs intentionally disagree with queue order.
				id := int64(10000 - i)
				for _, query := range []string{
					`INSERT INTO hg_content_profile VALUES(?,1,'public',NULL)`,
					`INSERT INTO hg_youban_publish_profile_state VALUES(?,1,0,NULL)`,
				} {
					if err := exec(query, id); err != nil {
						return err
					}
				}
				if err := exec(`INSERT INTO hg_youban_publish_channel_profile
					(tenant_id,account_id,channel_id,profile_id,status,batch_order) VALUES(1,0,10,?,'active',?)`, id, i); err != nil {
					return err
				}
			}
			return nil
		}
		if err := seed(1, 263); err != nil {
			return err
		}
		var fourth []cycleBatchSnapshotItem
		for day := 1; day <= 4; day++ {
			candidates, err := batchRotationCandidatesTx(ctx, tx, 1, 10, 70)
			if err != nil {
				return err
			}
			items, err := newCycleBatchSnapshot(1, 10, 70, candidates)
			if err != nil {
				return err
			}
			if len(items) != 70 {
				return fmt.Errorf("day %d selected %d, want 70", day, len(items))
			}
			if err := reserveBatchRotationCandidatesTx(ctx, tx, 1, 10, candidates); err != nil {
				return err
			}
			if err := persistCycleBatchSnapshotTx(ctx, tx, int64(day), items); err != nil {
				return err
			}
			if day == 4 {
				fourth = items
				for i, item := range items {
					queueIndex := 211 + i
					if i >= 53 {
						queueIndex = i - 52
					}
					if item.ProfileID != int64(10000-queueIndex) {
						return fmt.Errorf("day4 item%d=%d, expected 53 old +17 new round", i, item.ProfileID)
					}
				}
			}
			for _, item := range items {
				// Simulate successful callbacks moving members to the tail while
				// the immutable snapshot remains available for producer pagination.
				if err := exec(`UPDATE hg_youban_publish_channel_profile SET batch_order=nextval('hg_ybp_batch_order_seq') WHERE profile_id=?`,
					item.ProfileID); err != nil {
					return err
				}
			}
		}
		run := cycleRunRecord{Id: 4, TenantId: 1, ChannelId: 10}
		firstPage, err := cycleBatchSnapshotPageModel(tx.Model(publishCycleBatchItemTable).Ctx(ctx), run, 19)
		if err != nil {
			return err
		}
		replay, err := cycleBatchSnapshotPageModel(tx.Model(publishCycleBatchItemTable).Ctx(ctx), run, 19)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(firstPage, replay) {
			return fmt.Errorf("same cursor replay changed daily candidates")
		}
		var all []cycleBatchSnapshotItem
		for {
			page, err := cycleBatchSnapshotPageModel(tx.Model(publishCycleBatchItemTable).Ctx(ctx), run, 19)
			if err != nil {
				return err
			}
			if len(page) == 0 {
				break
			}
			all = append(all, page...)
			run.CursorId = page[len(page)-1].Ordinal
		}
		if len(all) != 70 {
			return fmt.Errorf("snapshot pagination after callback moves: %d", len(all))
		}
		for i := range all {
			if all[i].ProfileID != fourth[i].ProfileID || all[i].Ordinal != int64(i+1) {
				return fmt.Errorf("snapshot drift at %d", i)
			}
		}
		// Twenty newly admitted members join at the queue tail, including IDs
		// smaller than every old member. They cannot alter an existing snapshot.
		if err := seed(264, 283); err != nil {
			return err
		}
		if err := exec(`UPDATE hg_youban_publish_channel_profile SET batch_order=batch_order+10000 WHERE profile_id<=9736`); err != nil {
			return err
		}
		// Remove the oldest remaining profile from authoritative eligibility.
		if err := exec(`UPDATE hg_content_profile SET status=2 WHERE id=9982`); err != nil {
			return err
		}
		next, err := batchRotationCandidatesTx(ctx, tx, 1, 10, 70)
		if err != nil {
			return err
		}
		if len(next) != 70 || next[0].ProfileID != 9981 {
			return fmt.Errorf("new20/offline/descending IDs did not retain FIFO: %+v", next)
		}
		large, err := batchRotationCandidatesTx(ctx, tx, 1, 10, 400)
		if err != nil {
			return err
		}
		short, err := newCycleBatchSnapshot(1, 10, 400, large)
		if err != nil {
			return err
		}
		if len(short) != 282 {
			return fmt.Errorf("insufficient candidates must not wrap/repeat: got %d want282", len(short))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
