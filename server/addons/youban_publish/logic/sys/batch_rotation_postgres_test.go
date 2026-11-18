package sys

import (
	"context"
	"os"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// Run only against a disposable local PG. All tables below are transaction-local.
func TestBatchRotationPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_BATCH_ROTATION_TEST_PG_PORT")
	if port == "" {
		t.Skip("set YOUBAN_BATCH_ROTATION_TEST_PG_PORT for disposable localhost PG")
	}
	if port != "55491" {
		t.Fatal("isolated test requires explicit localhost port 55491")
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: "postgres", Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		exec := func(sql string) {
			t.Helper()
			if _, err := tx.Exec(sql); err != nil {
				t.Fatal(err)
			}
		}
		exec(`CREATE TEMP SEQUENCE hg_ybp_batch_order_seq`)
		exec(`CREATE TEMP TABLE hg_youban_publish_channel_profile (
			id bigserial PRIMARY KEY, tenant_id bigint,account_id bigint,channel_id bigint,profile_id bigint,
			status text,last_job_id bigint DEFAULT 0,batch_last_sent_job_id bigint DEFAULT 0,
			batch_order bigint DEFAULT 0,next_cycle_at timestamp,created_at timestamp,updated_at timestamp,
			UNIQUE(channel_id,profile_id))`)
		exec(`CREATE TEMP TABLE hg_content_profile (id bigint PRIMARY KEY,status int,visibility text,deleted_at timestamp)`)
		exec(`CREATE TEMP TABLE hg_youban_publish_profile_state
			(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp)`)
		exec(`CREATE TEMP TABLE hg_youban_publish_account
			(id bigint PRIMARY KEY,tenant_id bigint,status int,deleted_at timestamp)`)
		exec(`CREATE TEMP TABLE hg_youban_publish_profile_channel
			(tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,is_manual int,deleted_at timestamp)`)
		exec(`CREATE TEMP TABLE hg_youban_publish_channel
			(id bigint PRIMARY KEY,tenant_id bigint,status int,publish_direction text,is_default_selected int,deleted_at timestamp)`)
		exec(`INSERT INTO hg_content_profile VALUES
			(1,1,'public',NULL),(2,2,'public',NULL),(3,1,'private',NULL),(4,1,'public',NULL)`)
		exec(`INSERT INTO hg_youban_publish_profile_state VALUES
			(1,1,0,NULL),(2,1,0,NULL),(3,1,0,NULL),(4,1,0,NULL)`)
		exec(`INSERT INTO hg_youban_publish_channel VALUES (10,1,1,'up',1,NULL)`)
		s := new(sSysPublish)
		for _, id := range []int64{1, 2, 3, 4} {
			if err := s.admitBatchRotationProfileTx(ctx, tx, telegramJobRecord{
				Id: id, TenantId: 1, AccountId: 0, ChannelId: 10, ProfileId: id,
			}); err != nil {
				return err
			}
		}
		items, err := batchRotationCandidatesTx(ctx, tx, 1, 10, 1001)
		if err != nil {
			return err
		}
		if len(items) != 2 || items[0].ProfileID != 1 || items[1].ProfileID != 4 {
			t.Fatalf("unsent BOT candidates: %+v", items)
		}
		order := items[0].Order
		exec(`UPDATE hg_content_profile SET status=2 WHERE id=1`)
		items, err = batchRotationCandidatesTx(ctx, tx, 1, 10, 1)
		if err != nil {
			return err
		}
		if len(items) != 1 || items[0].ProfileID != 4 {
			t.Fatalf("rejected keyset head hid later eligible profile: %+v", items)
		}
		exec(`UPDATE hg_content_profile SET status=1 WHERE id=1`)
		exec(`UPDATE hg_youban_publish_channel_profile SET last_job_id=99 WHERE profile_id=1`)
		if err := s.admitBatchRotationProfileTx(ctx, tx, telegramJobRecord{
			Id: 100, TenantId: 1, AccountId: 0, ChannelId: 10, ProfileId: 1,
		}); err != nil {
			return err
		}
		row, err := tx.GetOne(`SELECT batch_order,last_job_id FROM hg_youban_publish_channel_profile WHERE profile_id=1`)
		if err != nil {
			return err
		}
		if row["batch_order"].Int64() != order || row["last_job_id"].Int64() != 99 {
			t.Fatal("admission moved an existing member or overwrote last_job_id")
		}
		exec(`UPDATE hg_youban_publish_channel_profile
			SET status='inactive',batch_order=0,last_job_id=99,batch_last_sent_job_id=101 WHERE profile_id=1`)
		assertInactive := func() error {
			t.Helper()
			current, err := tx.GetOne(`SELECT status,batch_order,last_job_id,batch_last_sent_job_id
				FROM hg_youban_publish_channel_profile WHERE profile_id=1`)
			if err != nil {
				return err
			}
			if current["status"].String() != "inactive" || current["batch_order"].Int64() != 0 {
				t.Fatalf("stale or ineligible admission reactivated member: %+v", current)
			}
			return nil
		}
		// Both watermarks are strict: equality with either one must reject.
		for _, id := range []int64{98, 99, 100, 101} {
			if err := s.admitBatchRotationProfileTx(ctx, tx, telegramJobRecord{
				Id: id, TenantId: 1, AccountId: 0, ChannelId: 10, ProfileId: 1,
			}); err != nil {
				return err
			}
			if err := assertInactive(); err != nil {
				return err
			}
		}
		exec(`UPDATE hg_youban_publish_channel_profile SET last_job_id=103 WHERE profile_id=1`)
		for _, id := range []int64{102, 103} {
			if err := s.admitBatchRotationProfileTx(ctx, tx, telegramJobRecord{
				Id: id, TenantId: 1, AccountId: 0, ChannelId: 10, ProfileId: 1,
			}); err != nil {
				return err
			}
			if err := assertInactive(); err != nil {
				return err
			}
		}
		exec(`UPDATE hg_content_profile SET visibility='private' WHERE id=1`)
		if err := s.admitBatchRotationProfileTx(ctx, tx, telegramJobRecord{
			Id: 104, TenantId: 1, AccountId: 0, ChannelId: 10, ProfileId: 1,
		}); err != nil {
			return err
		}
		if err := assertInactive(); err != nil {
			return err
		}
		exec(`UPDATE hg_content_profile SET visibility='public' WHERE id=1`)
		if err := s.admitBatchRotationProfileTx(ctx, tx, telegramJobRecord{
			Id: 104, TenantId: 1, AccountId: 0, ChannelId: 10, ProfileId: 1,
		}); err != nil {
			return err
		}
		row, err = tx.GetOne(`SELECT status,batch_order,last_job_id,batch_last_sent_job_id
			FROM hg_youban_publish_channel_profile WHERE profile_id=1`)
		if err != nil {
			return err
		}
		if row["status"].String() != "active" || row["batch_order"].Int64() <= 0 {
			t.Fatal("explicit new job did not readmit inactive member")
		}
		if row["last_job_id"].Int64() != 103 || row["batch_last_sent_job_id"].Int64() != 101 {
			t.Fatal("readmission changed publication watermarks")
		}
		exec(`UPDATE hg_youban_publish_channel_profile SET status='inactive' WHERE profile_id=1`)
		sent := telegramJobRecord{Id: 110, TenantId: 1, AccountId: 0, ChannelId: 10, ProfileId: 4}
		if err := s.upsertChannelProfileFromJobTx(ctx, tx, sent); err != nil {
			return err
		}
		before, err := tx.GetValue(`SELECT batch_order FROM hg_youban_publish_channel_profile WHERE profile_id=4`)
		if err != nil {
			return err
		}
		for _, id := range []int64{110, 109} {
			sent.Id = id
			if err := s.upsertChannelProfileFromJobTx(ctx, tx, sent); err != nil {
				return err
			}
		}
		after, err := tx.GetValue(`SELECT batch_order FROM hg_youban_publish_channel_profile WHERE profile_id=4`)
		if err != nil {
			return err
		}
		if before.Int64() != after.Int64() {
			t.Fatal("duplicate or late success moved queue")
		}
		exec(`UPDATE hg_youban_publish_channel_profile SET status='inactive' WHERE profile_id=4`)
		sent.Id = 111
		if err := s.upsertChannelProfileFromJobTx(ctx, tx, sent); err != nil {
			return err
		}
		row, err = tx.GetOne(`SELECT status,batch_order FROM hg_youban_publish_channel_profile WHERE profile_id=4`)
		if err != nil {
			return err
		}
		if row["status"].String() != "inactive" || row["batch_order"].Int64() != before.Int64() {
			t.Fatal("late success reactivated inactive member")
		}
		// Rejected head must not hide eligible later rows when page limit is 1.
		exec(`UPDATE hg_content_profile SET status=2 WHERE id=4`)
		items, err = batchRotationCandidatesTx(ctx, tx, 1, 10, 1)
		if err != nil {
			return err
		}
		if len(items) != 0 {
			t.Fatalf("offline profile returned: %+v", items)
		}

		// A separate channel contains more than the supported limit, so these
		// assertions detect accidental truncation at the old limit of 1000.
		exec(`INSERT INTO hg_content_profile
			SELECT id,1,'public',NULL FROM generate_series(10000,15000) AS id`)
		exec(`INSERT INTO hg_youban_publish_profile_state
			SELECT id,1,0,NULL FROM generate_series(10000,15000) AS id`)
		exec(`INSERT INTO hg_youban_publish_channel VALUES (20,1,1,'up',1,NULL)`)
		exec(`INSERT INTO hg_youban_publish_channel_profile
			(tenant_id,account_id,channel_id,profile_id,status,batch_order,last_job_id,batch_last_sent_job_id)
			SELECT 1,0,20,id,'active',nextval('hg_ybp_batch_order_seq'),7,7
			FROM generate_series(10000,15000) AS id ORDER BY id`)
		var candidates []batchRotationCandidate
		for _, limit := range []int{5000, 5001} {
			candidates, err = batchRotationCandidatesTx(ctx, tx, 1, 20, limit)
			if err != nil {
				return err
			}
			if len(candidates) != 5000 {
				t.Fatalf("limit=%d returned %d candidates, want 5000", limit, len(candidates))
			}
			for i := 1; i < len(candidates); i++ {
				if candidates[i].Order <= candidates[i-1].Order {
					t.Fatalf("limit=%d returned unordered candidates at %d", limit, i)
				}
			}
		}
		exec(`CREATE TEMP TABLE batch_rotation_test_snapshot(profile_id bigint PRIMARY KEY,original_order bigint)`)
		exec(`SAVEPOINT rotation_over_limit`)
		tooMany := append(append([]batchRotationCandidate{}, candidates...), candidates[0])
		if err := reserveBatchRotationCandidatesTx(ctx, tx, 1, 20, tooMany); err == nil {
			t.Fatal("reserve accepted more than 5000 candidates")
		}
		exec(`ROLLBACK TO SAVEPOINT rotation_over_limit`)
		exec(`RELEASE SAVEPOINT rotation_over_limit`)

		// Simulate another claimant changing one order after the snapshot read.
		// The other candidate still matches, so UPDATE can partially succeed;
		// rolling back the caller transaction must undo BOTH snapshot and move.
		first, stale := candidates[0], candidates[1]
		if _, err := tx.Exec(`UPDATE hg_youban_publish_channel_profile
			SET batch_order=nextval('hg_ybp_batch_order_seq')
			WHERE channel_id=20 AND profile_id=?`, stale.ProfileID); err != nil {
			return err
		}
		exec(`SAVEPOINT rotation_claim`)
		if _, err := tx.Exec(`INSERT INTO batch_rotation_test_snapshot VALUES (?,?),(?,?)`,
			first.ProfileID, first.Order, stale.ProfileID, stale.Order); err != nil {
			return err
		}
		if err := reserveBatchRotationCandidatesTx(ctx, tx, 1, 20, []batchRotationCandidate{first, stale}); err == nil {
			t.Fatal("reserve accepted a stale original order")
		}
		partiallyMoved, err := tx.GetValue(`SELECT batch_order FROM hg_youban_publish_channel_profile
			WHERE channel_id=20 AND profile_id=?`, first.ProfileID)
		if err != nil {
			return err
		}
		if partiallyMoved.Int64() == first.Order {
			t.Fatal("rollback fixture did not exercise a partially matched reservation")
		}
		exec(`ROLLBACK TO SAVEPOINT rotation_claim`)
		exec(`RELEASE SAVEPOINT rotation_claim`)
		snapshotCount, err := tx.GetValue(`SELECT count(*) FROM batch_rotation_test_snapshot`)
		if err != nil {
			return err
		}
		current, err := tx.GetOne(`SELECT batch_order,last_job_id,batch_last_sent_job_id,status
			FROM hg_youban_publish_channel_profile WHERE channel_id=20 AND profile_id=?`, first.ProfileID)
		if err != nil {
			return err
		}
		if snapshotCount.Int() != 0 || current["batch_order"].Int64() != first.Order {
			t.Fatal("caller rollback did not restore snapshot and matched candidate order")
		}
		// Re-read fresh candidates and accept the full 5000-item reservation.
		candidates, err = batchRotationCandidatesTx(ctx, tx, 1, 20, 5000)
		if err != nil {
			return err
		}
		if err := reserveBatchRotationCandidatesTx(ctx, tx, 1, 20, candidates); err != nil {
			return err
		}
		unchanged, err := tx.GetValue(`SELECT count(*) FROM hg_youban_publish_channel_profile
			WHERE channel_id=20 AND status='active' AND last_job_id=7 AND batch_last_sent_job_id=7`)
		if err != nil {
			return err
		}
		if unchanged.Int() != 5001 {
			t.Fatal("reserve changed publication state or success watermarks")
		}
		for _, candidate := range candidates[:2] {
			newOrder, err := tx.GetValue(`SELECT batch_order FROM hg_youban_publish_channel_profile
				WHERE channel_id=20 AND profile_id=?`, candidate.ProfileID)
			if err != nil {
				return err
			}
			if newOrder.Int64() <= candidate.Order {
				t.Fatalf("reserve did not move profile %d to tail", candidate.ProfileID)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
