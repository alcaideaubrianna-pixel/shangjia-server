package sys

import (
	"context"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"
)

// Opt-in through the shared YOUBAN_LOCAL_PUBLISH_RECORD loopback fixture.
// Log IDs, legacy IDs and JobIDs intentionally differ.
func TestPublishRecordBatchRankStableAcrossKeywordAndPagesPostgres(t *testing.T) {
	db := publishRecordReadLocalDB(t)
	ctx := context.Background()
	for _, query := range []string{
		"ALTER TABLE " + publishTgJobTable + " ADD COLUMN operation_no text",
		"CREATE TABLE " + publishFullPushBatchTable + " (batch_no text PRIMARY KEY,total_count integer,queued_count integer)",
		"CREATE TABLE " + publishSuccessRecordTable + " (id bigint PRIMARY KEY,job_id bigint UNIQUE,operation_no text,status text,created_at timestamp)",
		"INSERT INTO " + publishFullPushBatchTable + " VALUES ('full_push:1:batch',10,8),('full_push:2:batch',5,4)",
		"INSERT INTO " + publishTgJobTable + ` (id,tenant_id,status,error_message,operation_no) VALUES
			(101,10,'sent','','full_push:1:batch:999'),
			(102,10,'sent','','full_push:1:batch:001'),
			(103,10,'sent','','full_push:1:batch:002'),
			(201,10,'sent','','full_push:2:batch:001')`,
		"INSERT INTO " + publishSuccessRecordTable + ` VALUES
			(901,101,'full_push:1:batch:999','success','2026-11-17 10:00:00'),
			(100,102,'full_push:1:batch:001','success','2026-11-17 10:01:00'),
			(500,103,'full_push:1:batch:002','success','2026-11-17 10:01:00'),
			(902,201,'full_push:2:batch:001','success','2026-11-17 09:00:00')`,
		"INSERT INTO " + publishRecordLogTable + `
			(id,job_id,tenant_id,account_id,profile_id,operation_no,action,status,message,title) VALUES
			(5001,101,10,11,501,'full_push:1:batch:999','full_push','success','done','first'),
			(9001,102,10,11,502,'full_push:1:batch:001','full_push','success','done','second'),
			(7001,103,10,11,503,'full_push:1:batch:002','full_push','success','done','rank-target'),
			(2000,201,10,11,601,'full_push:2:batch:001','full_push','success','done','other-batch')`,
	} {
		if _, err := db.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	s := new(sSysPublish)
	for _, tc := range []struct {
		name    string
		keyword string
		page    int
		perPage int
		count   int
	}{
		{"complete list", "", 1, 50, 4},
		{"first page", "", 1, 2, 4},
		{"second single-row page", "", 2, 1, 4},
		{"keyword isolates third success", "rank-target", 1, 50, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := &sysin.PublishRecordListInp{Keyword: tc.keyword}
			in.Page, in.PerPage = tc.page, tc.perPage
			rows, count, err := s.publishRecordList(ctx, in, 10, 11)
			if err != nil || count != tc.count {
				t.Fatalf("count=%d want=%d err=%v", count, tc.count, err)
			}
			found := false
			for _, row := range rows {
				want := map[int64]int{101: 1, 102: 2, 103: 3, 201: 1}[row.JobId]
				if row.ProgressDone != want {
					t.Fatalf("job=%d rank=%d want=%d", row.JobId, row.ProgressDone, want)
				}
				if row.JobId == 103 {
					found = true
					if row.ProgressTotal != 10 || row.ProgressQueued != 8 || row.ProgressText != "3/10（已调度8）" {
						t.Fatalf("batch totals/text changed: %+v", row)
					}
				}
			}
			if !found {
				t.Fatal("target JobID 103 missing from test page")
			}
		})
	}
}
