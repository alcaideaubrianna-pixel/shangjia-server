package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	"hotgo/addons/youban_publish/model"
)

// Uses the opt-in YOUBAN_LOCAL_TG_OUTBOX loopback fixture (127.0.0.1:35432).
// No production DSN, Redis, consumer or sender is involved.
func TestPublishRecordWriteStatusAndImmutableSnapshotPostgres(t *testing.T) {
	db := telegramOutboxRetryLocalDB(t)
	ctx := context.Background()
	ddl := []string{
		`CREATE TABLE ` + publishSuccessRecordTable + ` (
			id bigserial PRIMARY KEY, job_id bigint UNIQUE, task_id bigint,
			tenant_id bigint, account_id bigint, profile_id bigint, channel_id bigint,
			bot_id bigint, operation_no text, target_chat_id text, action text,
			status text, message text, created_at timestamp)`,
		`CREATE TABLE ` + publishAccountTable + ` (
			id bigint PRIMARY KEY, tenant_id bigint, nickname text, username text,
			account_type text, deleted_at timestamp)`,
		`CREATE TABLE ` + publishTenantTable + ` (id bigint PRIMARY KEY, name text, deleted_at timestamp)`,
		`CREATE TABLE ` + publishChannelTable + ` (id bigint PRIMARY KEY, channel_title text, channel_username text)`,
		`CREATE TABLE ` + publishBotTable + ` (id bigint PRIMARY KEY, bot_name text, bot_username text)`,
	}
	for _, query := range ddl {
		if _, err := db.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile("../../resource/sql/publish_record_log.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range strings.Split(string(data), ";") {
		if strings.TrimSpace(query) == "" {
			continue
		}
		if _, err := db.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		"INSERT INTO " + publishAccountTable + " VALUES (1,1,'account-before','owner-before','admin',NULL)",
		"INSERT INTO " + publishTenantTable + " VALUES (1,'tenant-before',NULL)",
		"INSERT INTO " + publishChannelTable + " VALUES (1,'channel-before','channel_user_before')",
		"INSERT INTO " + publishBotTable + " VALUES (1,'bot-before','bot_user_before')",
	} {
		if _, err := db.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	s := new(sSysPublish)
	job := telegramJobRecord{
		Id: 1, TaskId: 1, TenantId: 1, AccountId: 1, ProfileId: 1,
		ChannelId: 1, BotId: 1, OperationNo: "profile:1", TargetChatId: "-1001",
		TitleSnapshot: "title-before",
	}
	if err := s.upsertPublishJobRecord(ctx, job, model.PublishRecordStatusPending, "pending-before"); err != nil {
		t.Fatal(err)
	}
	// A fixed old timestamp makes preservation and updated_at movement
	// observable without sleeps or dependence on timestamp clock resolution.
	for _, query := range []string{
		"UPDATE " + publishSuccessRecordTable + " SET created_at='2000-01-01 00:00:00'",
		"UPDATE " + publishRecordLogTable + " SET created_at='2000-01-01 00:00:00+00',updated_at='2000-01-01 00:00:00+00'",
	} {
		if _, err := db.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	before, err := db.GetOne(ctx, "SELECT * FROM "+publishRecordLogTable+" WHERE job_id=1")
	if err != nil {
		t.Fatal(err)
	}
	for field, expected := range map[string]string{
		"status": model.PublishRecordStatusPending.String(), "title": "title-before",
		"account_name": "account-before", "tenant_name": "owner-before",
		"channel_title": "channel-before", "channel_username": "channel_user_before",
		"bot_name": "bot-before", "bot_username": "bot_user_before",
	} {
		if before[field].String() != expected {
			t.Fatalf("initial snapshot %s=%q, want %q", field, before[field].String(), expected)
		}
	}
	for i, status := range []model.PublishRecordStatus{model.PublishRecordStatusSending, model.PublishRecordStatusSuccess} {
		job.TaskId, job.TenantId, job.AccountId, job.ProfileId = 2, 2, 2, 2
		job.ChannelId, job.BotId = 2, 2
		job.OperationNo, job.TargetChatId, job.TitleSnapshot = "collect:changed", "-1002", "title-after"
		job.RetryCount = i + 1
		if err := s.upsertPublishJobRecord(ctx, job, status, status.String()+"-after"); err != nil {
			t.Fatal(err)
		}
		after, err := db.GetOne(ctx, "SELECT * FROM "+publishRecordLogTable+" WHERE job_id=1")
		if err != nil {
			t.Fatal(err)
		}
		if after["status"].String() != status.String() || after["message"].String() != status.String()+"-after" || after["retry_count"].Int() != job.RetryCount {
			t.Fatalf("delivery fields did not advance: %v", after)
		}
		for field, expected := range before {
			switch field {
			case "status", "message", "retry_count", "updated_at":
				continue
			}
			if after[field].String() != expected.String() {
				t.Fatalf("snapshot field %s changed: before=%v after=%v", field, expected, after[field])
			}
		}
		if after["updated_at"].String() == before["updated_at"].String() {
			t.Fatal("updated_at did not advance")
		}
		legacy, err := db.GetOne(ctx, "SELECT status,created_at FROM "+publishSuccessRecordTable+" WHERE job_id=1")
		if err != nil {
			t.Fatal(err)
		}
		if legacy["status"].String() != status.String() || legacy["created_at"].GTime().Time.Year() != 2000 {
			t.Fatalf("legacy status/created_at mismatch: %v", legacy)
		}
	}
	for _, table := range []string{publishSuccessRecordTable, publishRecordLogTable} {
		count, err := db.GetValue(ctx, "SELECT COUNT(*) FROM "+table+" WHERE job_id=1")
		if err != nil || count.Int() != 1 {
			t.Fatalf("upsert duplicated record in %s: count=%v err=%v", table, count, err)
		}
	}
}
