package sys

import (
	"context"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_publish/model/input/sysin"
	"os"
	"testing"
	"time"
)

func TestFullPushRecordsFollowExecutionPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable localhost:55484 PostgreSQL")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, q := range []string{
			"CREATE TEMP TABLE " + publishSuccessRecordTable + "(id bigserial PRIMARY KEY,job_id bigint UNIQUE,task_id bigint,tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,bot_id bigint,operation_no text,target_chat_id text,action text,status text,message text,created_at timestamp)",
			"CREATE TEMP TABLE " + publishTgJobTable + "(id bigint PRIMARY KEY,status text,error_message text)",
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		s := new(sSysPublish)
		jobs := make([]telegramJobRecord, 50)
		for i := range jobs {
			id := int64(i + 1)
			jobs[i] = telegramJobRecord{Id: id, TenantId: 1, AccountId: 11, ProfileId: id, ChannelId: 3, OperationNo: fmt.Sprintf("full_push:3:batch:%d", id)}
			if _, err := tx.Exec("INSERT INTO "+publishTgJobTable+" VALUES (?,'pending','')", id); err != nil {
				return err
			}
			if err := s.upsertPublishJobRecord(ctx, jobs[i], "pending", ""); err != nil {
				return err
			}
		}
		assertCount := func(want int) error {
			count, err := tx.GetValue("SELECT count(*) FROM " + publishSuccessRecordTable)
			if err != nil {
				return err
			}
			if count.Int() != want {
				t.Fatalf("record count=%d want %d", count.Int(), want)
			}
			return nil
		}
		if err := assertCount(0); err != nil {
			return err
		}
		for _, state := range []string{"sending", "failed_retry", "sending", "success"} {
			if err := s.upsertPublishJobRecord(ctx, jobs[0], state, ""); err != nil {
				return err
			}
			if err := assertCount(1); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("UPDATE " + publishTgJobTable + " SET status='sent' WHERE id=1"); err != nil {
			return err
		}
		if err := s.upsertPublishJobRecord(ctx, jobs[1], "sending", ""); err != nil {
			return err
		}
		if err := s.upsertPublishJobRecord(ctx, jobs[2], "failed", "发送失败"); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE " + publishTgJobTable + " SET status='failed' WHERE id=3"); err != nil {
			return err
		}
		// Returning to pending preserves an already-created execution record.
		if err := s.upsertPublishJobRecord(ctx, jobs[1], "pending", ""); err != nil {
			return err
		}
		if err := assertCount(3); err != nil {
			return err
		}
		for _, q := range []string{
			"INSERT INTO " + publishSuccessRecordTable + " (job_id,tenant_id,account_id,profile_id,operation_no,action,status,message) VALUES (4,1,11,4,'full_push:3:batch:4','full_push','pending','needle'),(5,1,11,5,'full_push:3:batch:5','publish','pending','needle'),(6,1,11,6,'full_push:3:batch:6','full_push','pending','needle'),(999,1,11,999,'full_push:3:batch:999','full_push','pending','needle'),(7,1,11,7,'profile:7','profile_publish','pending','needle'),(8,2,22,8,'profile:8','profile_publish','success','needle')",
			"UPDATE " + publishTgJobTable + " SET status='sent' WHERE id=6",
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		for _, keyword := range []string{"", "needle"} {
			base := g.DB().Model(publishSuccessRecordTable+" l").Safe().Ctx(ctx).Where("l.tenant_id", 1)
			if keyword == "" {
				base = applyPublishRecordVisibilityFilter(base)
			} else {
				base = base.LeftJoin(publishTgJobTable+" j", "j.id=l.job_id").LeftJoin("(SELECT '' AS title) p", "TRUE").LeftJoin("(SELECT '' AS channel_title) c", "TRUE").LeftJoin("(SELECT '' AS bot_name,'' AS bot_username) b", "TRUE").LeftJoin("(SELECT '' AS nickname) a", "TRUE")
				base = s.applyPublishRecordFilters(base, &sysin.PublishRecordListInp{Keyword: keyword}, 11)
			}
			count, err := base.Clone().Count()
			if err != nil {
				return err
			}
			want := 5
			if keyword != "" {
				want = 2
			}
			if count != want {
				t.Fatalf("keyword=%q count=%d want %d", keyword, count, want)
			}
			rows, err := base.Clone().Fields("l.job_id").OrderAsc("l.job_id").Page(1, 2).All()
			if err != nil {
				return err
			}
			if len(rows) != 2 {
				t.Fatalf("pagination returned %d rows", len(rows))
			}
			for _, row := range rows {
				if id := row["job_id"].Int64(); id == 4 || id == 5 || id == 999 {
					t.Fatalf("unexecuted full-push leaked: %d", id)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
