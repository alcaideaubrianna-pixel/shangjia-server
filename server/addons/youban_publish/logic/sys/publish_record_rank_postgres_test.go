package sys

import (
	"context"
	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
)

// No batch/success tables exist: listing full-push records must not query them.
func TestPublishRecordRecentPagesWithoutBatchRankingPostgres(t *testing.T) {
	db := publishRecordReadLocalDB(t)
	ctx := context.Background()
	for _, query := range []string{
		"INSERT INTO " + publishTgJobTable + " VALUES (1,10,'sent',''),(2,10,'failed','actual failure')",
		"INSERT INTO " + publishRecordLogTable + ` (id,job_id,tenant_id,account_id,profile_id,operation_no,action,status,message,created_at)
   SELECT n,n,10,11,501,'full_push:1:batch:' || n,'full_push','sending','waiting',
   TIMESTAMP '2026-11-18 10:00:00' + ((22-n)/2)*INTERVAL '1 minute'
   FROM generate_series(1,21) n`,
		"INSERT INTO " + publishRecordLogTable + ` (id,job_id,tenant_id,account_id,profile_id,status,message) VALUES
   (100,100,20,11,501,'success','other tenant'),
   (101,101,10,12,501,'success','other account'),
   (102,102,10,11,502,'success','other profile')`,
	} {
		if _, err := db.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	s := new(sSysPublish)
	seen := make(map[int64]bool)
	for page := 1; page <= 3; page++ {
		in := &sysin.PublishRecordListInp{ProfileId: 501, AccountId: 12}
		in.Page = page
		if err := in.Filter(ctx); err != nil {
			t.Fatal(err)
		}
		rows, total, err := s.publishRecordList(ctx, in, 10, 11)
		wantLen := 10
		if page == 3 {
			wantLen = 1
		}
		if err != nil || total != 21 || len(rows) != wantLen {
			t.Fatalf("page=%d total=%d rows=%d err=%v", page, total, len(rows), err)
		}
		for i, row := range rows {
			position := (page-1)*10 + i
			wantID := int64(position + 1)
			if position < 20 {
				wantID = int64((position/2)*2 + 2 - position%2)
			}
			if row.Id != wantID || seen[row.Id] || row.AccountId != 11 || row.ProfileId != 501 {
				t.Fatalf("position=%d wantID=%d row=%+v duplicate=%v", position, wantID, row, seen[row.Id])
			}
			seen[row.Id] = true
			if row.ProgressDone != 0 || row.ProgressText != "" {
				t.Fatalf("unexpected batch ranking: %+v", row)
			}
			if row.JobId == 1 && row.Status != publishmodel.PublishRecordStatusSuccess {
				t.Fatalf("stale success status: %+v", row)
			}
			if row.JobId == 2 && (row.Status != publishmodel.PublishRecordStatusFailed || row.Message != "actual failure") {
				t.Fatalf("stale failure status: %+v", row)
			}
		}
	}
}
