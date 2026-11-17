package sys

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func publishRecordReadLocalDB(t *testing.T) gdb.DB {
	t.Helper()
	if os.Getenv("YOUBAN_LOCAL_PUBLISH_RECORD") != "1" {
		t.Skip("requires disposable PostgreSQL on 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}
	root, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	schema := fmt.Sprintf("publish_record_read_%d", time.Now().UnixNano())
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	db := g.DB()
	for _, sql := range []string{
		`CREATE TABLE ` + publishTgJobTable + ` (
			id bigint PRIMARY KEY, tenant_id bigint, status text, error_message text)`,
		`CREATE TABLE ` + publishRecordLogTable + ` (
			id bigint PRIMARY KEY, job_id bigint UNIQUE, task_id bigint DEFAULT 0,
			tenant_id bigint, account_id bigint, profile_id bigint, channel_id bigint DEFAULT 1,
			bot_id bigint DEFAULT 1, operation_no text DEFAULT 'profile:test',
			target_chat_id text DEFAULT '-1001', action text DEFAULT 'profile_publish',
			status text, message text, created_at timestamp DEFAULT LOCALTIMESTAMP,
			title text DEFAULT 'needle', account_name text DEFAULT 'account',
			tenant_name text DEFAULT 'tenant', bot_name text DEFAULT 'bot',
			bot_username text DEFAULT 'bot', channel_title text DEFAULT 'channel',
			channel_username text DEFAULT 'channel')`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestPublishRecordReadUsesJobForSearchAndFiltersPostgres(t *testing.T) {
	db := publishRecordReadLocalDB(t)
	ctx := context.Background()
	for _, sql := range []string{
		`INSERT INTO ` + publishTgJobTable + ` VALUES
			(1,10,'sent',''),(2,10,'superseded','replaced'),
			(3,10,'failed','actual failure'),(4,10,'invalid_state',''),
			(99,20,'failed','private other tenant error')`,
		`INSERT INTO ` + publishRecordLogTable + `
			(id,job_id,tenant_id,account_id,profile_id,status,message) VALUES
			(101,1,10,11,501,'pending','waiting'),
			(102,2,10,11,501,'sending','sending'),
			(103,3,10,11,501,'pending','waiting'),
			(104,4,10,11,501,'pending','waiting'),
			(105,5,10,11,501,'sent','legacy success'),
			(106,99,10,11,501,'pending','local snapshot'),
			(107,7,20,22,501,'success','other tenant')`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	s := &sSysPublish{}
	for _, keyword := range []string{"", "needle", "501"} {
		in := &sysin.PublishRecordListInp{ProfileId: 501, Keyword: keyword}
		in.Page, in.PerPage = 1, 50
		rows, count, err := s.publishRecordList(ctx, in, 10, 11)
		if err != nil || count != 6 || len(rows) != 6 {
			t.Fatalf("keyword=%q count=%d rows=%v err=%v", keyword, count, rows, err)
		}
		got := make(map[int64]*sysin.PublishRecordModel)
		for _, row := range rows {
			got[row.JobId] = row
		}
		for jobID, want := range map[int64]publishmodel.PublishRecordStatus{
			1: publishmodel.PublishRecordStatusSuccess, 2: publishmodel.PublishRecordStatusSuperseded,
			3: publishmodel.PublishRecordStatusFailed, 4: publishmodel.PublishRecordStatusUnknown,
			5: publishmodel.PublishRecordStatusSuccess, 99: publishmodel.PublishRecordStatusPending,
		} {
			if got[jobID] == nil || got[jobID].Status != want {
				t.Fatalf("keyword=%q job=%d got=%v want=%s", keyword, jobID, got[jobID], want)
			}
		}
		if got[1].Message == "waiting" || got[3].Message != "actual failure" ||
			got[99].Message != "local snapshot" {
			t.Fatalf("messages are stale or crossed tenant boundary: %v", rows)
		}
		for status, want := range map[publishmodel.PublishRecordStatus]int{
			publishmodel.PublishRecordStatusSuccess: 2, publishmodel.PublishRecordStatusPending: 1,
			publishmodel.PublishRecordStatusFailed: 1, publishmodel.PublishRecordStatusUnknown: 1,
		} {
			in.Status = status
			filtered, count, err := s.publishRecordList(ctx, in, 10, 11)
			if err != nil || count != want || len(filtered) != want {
				t.Fatalf("keyword=%q status=%s count=%d want=%d err=%v", keyword, status, count, want, err)
			}
		}
	}
	for keyword, want := range map[string]int{"上架推送成功": 1, "waiting": 0} {
		in := &sysin.PublishRecordListInp{ProfileId: 501, Keyword: keyword}
		in.Page, in.PerPage = 1, 50
		rows, count, err := s.publishRecordList(ctx, in, 10, 11)
		if err != nil || count != want || len(rows) != want {
			t.Fatalf("display message search keyword=%q count=%d want=%d err=%v", keyword, count, want, err)
		}
	}
}

func TestPublishRecordSQLAndGoMappingAgreePostgres(t *testing.T) {
	db := publishRecordReadLocalDB(t)
	ctx := context.Background()
	for _, mapping := range publishRecordJobStatusMappings {
		result, err := db.GetValue(ctx, "SELECT "+publishRecordStatusExpression+
			" FROM (SELECT 1 AS id,?::text AS status) j CROSS JOIN (SELECT 'pending'::text AS status) l",
			mapping.Job.String())
		if err != nil || result.String() != publishRecordStatusFromJob(mapping.Job).String() {
			t.Fatalf("job=%s SQL=%v Go=%s err=%v", mapping.Job, result, publishRecordStatusFromJob(mapping.Job), err)
		}
		for _, action := range []string{
			publishSuccessTypeProfile, publishSuccessTypeQuick, publishSuccessTypeCollect,
			publishSuccessTypeCycle, publishSuccessTypeFull,
		} {
			result, err := db.GetValue(ctx, "SELECT "+publishRecordMessageExpression+
				" FROM (SELECT 1 AS id,?::text AS status,''::text AS error_message) j"+
				" CROSS JOIN (SELECT 'pending'::text AS status,?::text AS action,''::text AS message) l",
				mapping.Job.String(), action)
			want := publishJobRecordMessage(action, mapping.Record)
			if err != nil || result.String() != want {
				t.Fatalf("job=%s action=%s SQL message=%v want=%s err=%v", mapping.Job, action, result, want, err)
			}
		}
	}
}

func TestPublishRecordClearKeepsOtherAccountsAndJobsPostgres(t *testing.T) {
	db := publishRecordReadLocalDB(t)
	ctx := context.Background()
	for _, sql := range []string{
		`CREATE TABLE ` + publishSuccessRecordTable + ` (id bigint PRIMARY KEY,tenant_id bigint,account_id bigint)`,
		`INSERT INTO ` + publishSuccessRecordTable + ` VALUES (1,10,11),(2,10,12),(3,20,22)`,
		`INSERT INTO ` + publishRecordLogTable + `
			(id,job_id,tenant_id,account_id,profile_id,status,message) VALUES
			(1,1,10,11,501,'success','done'),
			(2,2,10,12,501,'success','done'),
			(3,3,20,22,501,'success','done')`,
		`INSERT INTO ` + publishTgJobTable + ` VALUES (1,10,'sent','')`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := (&sSysPublish{}).publishRecordClear(ctx, 10, 11); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{publishSuccessRecordTable, publishRecordLogTable} {
		rows, err := db.GetAll(ctx, "SELECT id FROM "+table+" ORDER BY id")
		if err != nil || len(rows) != 2 || rows[0]["id"].Int64() != 2 || rows[1]["id"].Int64() != 3 {
			t.Fatalf("clear crossed scope or left read model behind: %s %v %v", table, rows, err)
		}
	}
	count, err := db.GetValue(ctx, "SELECT count(*) FROM "+publishTgJobTable)
	if err != nil || count.Int() != 1 {
		t.Fatalf("clearing records changed publish jobs: %v %v", count, err)
	}
}
