package sys

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/service"
)

type aiOpsOfflineTestContent struct{ service.ISysContent }

func (*aiOpsOfflineTestContent) ClearHomeProfileCardsCache(context.Context) {}

// Isolated default DB/Redis singletons, real SQLite and real Asynq enqueue;
// no worker or Telegram client is started.
func TestAIOpsOfflineDeletedParentBoundary(t *testing.T) {
	if sqlReviewSubprocess(t) {
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "sqlite", Name: filepath.Join(t.TempDir(), "offline.db")}})
	r := miniredis.RunT(t)
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: memory\nredis:\n  default:\n    address: '" + r.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", "legacy")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cache.SetAdapter(ctx)
	service.RegisterSysContent(&aiOpsOfflineTestContent{})
	execSQL := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := g.DB().Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	execSQL(`CREATE TABLE hg_content_profile(id INTEGER PRIMARY KEY,profile_no text,status integer,visibility text,deleted_at datetime,updated_at datetime,created_at datetime,published_at datetime,source_type text,source_note_uuid text,title text,summary text,plain_text text,tag_params text,cup_size text,province text,city text,review_status integer)`)
	execSQL(`CREATE TABLE hg_youban_publish_profile_state(profile_id INTEGER PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at datetime,publish_task_status text,publish_task_updated_at datetime,updated_at datetime)`)
	execSQL(`CREATE TABLE hg_youban_publish_tg_message(id INTEGER PRIMARY KEY,job_id bigint,profile_id bigint,target_chat_id text,tg_message_id bigint,status text,deleted_at datetime)`)
	execSQL(`CREATE TABLE hg_youban_publish_tg_job(id INTEGER PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,operation_type text,channel_id bigint,priority integer,status text,collect_event_id bigint,asynq_task_id text,queue_name text,updated_at datetime)`)
	execSQL(`CREATE TABLE hg_youban_publish_collect_event(id INTEGER PRIMARY KEY,tenant_id bigint,received_at datetime,source_type text)`)
	execSQL(`CREATE TABLE hg_youban_publish_channel_profile(tenant_id bigint,profile_id bigint,status text,updated_at datetime)`)
	execSQL(`CREATE TABLE hg_youban_publish_note_index(tenant_id bigint,account_id bigint,profile_id bigint,uuid text,profile_no text,title text,summary text,plain_text text,tag text,province text,city text,status integer,visibility text,review_status integer,task_status text,published_at datetime,source_updated_at datetime,created_at datetime,updated_at datetime,deleted_at datetime,is_collected boolean,UNIQUE(tenant_id,account_id,profile_id))`)
	execSQL(`CREATE TABLE hg_youban_publish_note_search(tenant_id bigint,account_id bigint,profile_id bigint,is_collected boolean,status integer,search_text text,deleted_at datetime,updated_at datetime,UNIQUE(tenant_id,account_id,profile_id))`)
	execSQL(`INSERT INTO hg_youban_publish_profile_state(profile_id,tenant_id,account_id,publish_task_status) VALUES(759869,99,647,'')`)
	execSQL(`INSERT INTO hg_youban_publish_tg_job(id,tenant_id,account_id,profile_id,operation_type,channel_id,priority,status) VALUES(3006421,99,647,759869,'delete',1851,3,'sent')`)
	execSQL(`INSERT INTO hg_youban_publish_tg_message VALUES(1,3006421,759869,'-1003888545899',48908,'sent',NULL)`)
	s := &sSysPublish{}
	t.Cleanup(func() {
		if s.tgQueueClient != nil {
			_ = s.tgQueueClient.Close()
		}
	})
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: r.Addr()})
	defer inspector.Close()
	t.Run("deleted_parent_message_still_enqueues_without_faking_deleted", func(t *testing.T) {
		result, err := s.AIOpsTelegramMessageOffline(ctx, &sysin.AIOpsTelegramOfflineInp{
			Messages: []sysin.AIOpsTelegramMessageRefInp{{ChatId: "-1003888545899", MessageId: 48908}},
		})
		if err != nil || result.CleanupQueued != 1 || result.Failed != 0 || result.Items[0].Offline {
			t.Fatalf("orphan message blocked or parent falsely offlined: result=%+v err=%v", result, err)
		}
		info, err := inspector.GetTaskInfo(tgQueueNameCleanup, telegramCleanupTaskID(3006421))
		if err != nil || info.Type != tgTaskTypeCleanup {
			t.Fatalf("normal cleanup not queued: task=%+v err=%v", info, err)
		}
		status, err := g.DB().GetValue(ctx, `SELECT status FROM hg_youban_publish_tg_message WHERE id=1`)
		if err != nil || status.String() != "sent" {
			t.Fatalf("enqueue fabricated Telegram deletion: status=%v err=%v", status, err)
		}
	})
	t.Run("profile_id_without_parent_remains_not_found", func(t *testing.T) {
		result, err := s.AIOpsTelegramMessageOffline(ctx, &sysin.AIOpsTelegramOfflineInp{ProfileIds: []int64{759869, 999999}})
		if err != nil || result.NotFound != 2 || result.CleanupQueued != 0 {
			t.Fatalf("nonexistent profile fabricated evidence: result=%+v err=%v", result, err)
		}
	})
	t.Run("existing_parent_uses_normal_offline_path", func(t *testing.T) {
		execSQL(`INSERT INTO hg_content_profile(id,profile_no,status,visibility) VALUES(42,'present',1,'public')`)
		execSQL(`INSERT INTO hg_youban_publish_profile_state(profile_id,tenant_id,account_id,publish_task_status) VALUES(42,78,615,'')`)
		result, err := s.AIOpsTelegramMessageOffline(ctx, &sysin.AIOpsTelegramOfflineInp{ProfileIds: []int64{42}})
		if err != nil || !result.Items[0].Offline || result.Failed != 0 {
			t.Fatalf("existing profile not offlined: result=%+v err=%v", result, err)
		}
		row, err := g.DB().GetOne(ctx, `SELECT status,visibility FROM hg_content_profile WHERE id=42`)
		if err != nil || row["status"].Int() != 2 || row["visibility"].String() != "private" {
			t.Fatalf("normal offline state not written: row=%v err=%v", row, err)
		}
	})
	t.Run("database_error_is_not_missing_parent", func(t *testing.T) {
		execSQL(`ALTER TABLE hg_youban_publish_profile_state RENAME TO broken_profile_state`)
		_, err := s.AIOpsTelegramMessageOffline(ctx, &sysin.AIOpsTelegramOfflineInp{
			Messages: []sysin.AIOpsTelegramMessageRefInp{{ChatId: "-1003888545899", MessageId: 48908}},
		})
		if err == nil {
			t.Fatal("database failure allowed cleanup")
		}
		execSQL(`ALTER TABLE broken_profile_state RENAME TO hg_youban_publish_profile_state`)
	})
}
