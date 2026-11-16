package sys

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/internal/library/cache"
)

// Opt-in disposable loopback database, isolated from other tests by subprocess.
// Exercise the real paginated lookup and pair transaction, not copied SQL.
func TestCollectMaterialPairAcrossPagesLocalPostgres(t *testing.T) {
	runCollectMaterialPairLocalPostgres(t, "-1003954466959")
}

func TestCollectMaterialPairShortChannelLocalPostgres(t *testing.T) {
	runCollectMaterialPairLocalPostgres(t, "-10042")
}

func runCollectMaterialPairLocalPostgres(t *testing.T, chatID string) {
	t.Helper()
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	previousDB := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	redis := miniredis.RunT(t)
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + redis.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cache.SetAdapter(ctx)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		statements := []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_event(
 id bigint PRIMARY KEY,tenant_id bigint DEFAULT 1,account_id bigint DEFAULT 0,tg_account_id bigint DEFAULT 0,
 source_id bigint DEFAULT 1,source_chat_id text DEFAULT '-1003954466959',source_message_id bigint,source_grouped_id text DEFAULT '',
 raw_text text DEFAULT '',media_count int DEFAULT 1,media_json text DEFAULT '[]',status text DEFAULT 'pending',material_role text DEFAULT 'pending',material_group_status text DEFAULT '',
 material_parent_event_id bigint DEFAULT 0,error_message text DEFAULT '',processed_at timestamp,
 received_at timestamp DEFAULT LOCALTIMESTAMP,created_at timestamp DEFAULT LOCALTIMESTAMP,updated_at timestamp DEFAULT LOCALTIMESTAMP) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_event_log(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,event_id bigint,stage text,status text,message text,meta_text text,created_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,event_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_origin(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,event_id bigint) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,raw_text,material_role) VALUES(5000,99,'这是一份尚未入库、错误关联了验证的旧展示资料，需要安全修复','display')`,
			`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,raw_text) VALUES(1,100,'这是一份展示资料，明确超过二十个字符，不能和下一份展示资料混合')`,
			`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,raw_text) SELECT 1000+n,101+n,'✅提交成功！' FROM generate_series(0,100) n`,
			`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,source_grouped_id,material_parent_event_id,material_role) VALUES(200,202,'verify-first',5000,'verify')`,
			`INSERT INTO hg_youban_publish_collect_event(id,tenant_id,source_message_id,raw_text) VALUES(3000,99,201,'这是另一个租户的展示资料，绝对不能成为当前验证资料的父级')`,
		}
		// The production media projection is intentionally explicit. Keep the
		// fixture complete as columns are added, without altering generated DAO.
		columns := make([]string, 0)
		for _, field := range strings.Split(collectEventMediaQueryFields(), ",") {
			typeName := "text"
			switch field {
			case "id":
				typeName = "bigint PRIMARY KEY"
			case "event_id":
				typeName = "bigint"
			case "created_at", "updated_at", "next_retry_at":
				typeName = "timestamp"
			}
			columns = append(columns, field+" "+typeName)
		}
		statements = append(statements,
			"CREATE TEMP TABLE hg_youban_publish_collect_event_media("+strings.Join(columns, ",")+") ON COMMIT DROP",
			`INSERT INTO hg_youban_publish_collect_event_media(id,event_id,media_type,source_file_id,cache_status) VALUES(1,1,'photo','photo','ready'),(2,200,'video','video','ready'),(3,3000,'photo','other','ready')`)
		for _, statement := range statements {
			if _, err := tx.Exec(strings.ReplaceAll(statement, "-1003954466959", chatID)); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		verify, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=200`)
		if err != nil {
			return err
		}
		service := NewSysPublish()
		if _, err := service.findStoredCollectDisplayEvent(ctx, verify); !isCollectProcessRetryError(err) {
			t.Fatalf("first page must retain work for the next page: %v", err)
		}
		// Future ingress must not reset progress past the first 100 noise rows.
		if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,source_chat_id,raw_text) VALUES(9000,9000,?,'这是不影响前序配对的后续展示资料，文案超过二十字')`, chatID); err != nil {
			return err
		}
		payload := collectProcessQueuePayload{TenantId: 1, SourceId: 1, SourceChatId: chatID}
		if err := dirtyCollectTargetBatch(ctx, payload); err != nil {
			return err
		}
		display, err := service.findStoredCollectDisplayEvent(ctx, verify)
		if err != nil {
			return err
		}
		if display["id"].Int64() != 1 {
			t.Fatalf("wrong or missing scoped display: %v", display)
		}
		// Unrelated future traffic changes the dirty generation but must not
		// prevent an already established finite interval from being bound.
		if _, err := tx.Ctx(ctx).Exec(`UPDATE hg_youban_publish_collect_event SET raw_text='这是被编辑成另一份展示的新文案，旧视频分类快照必须失效',updated_at=clock_timestamp() WHERE id=200`); err != nil {
			return err
		}
		if _, err := service.bindCollectMaterialPair(ctx, display, verify); !isCollectProcessRetryError(err) {
			t.Fatalf("edited verification reused a stale classification: %v", err)
		}
		if _, err := tx.Ctx(ctx).Exec(`UPDATE hg_youban_publish_collect_event SET raw_text='',updated_at=clock_timestamp() WHERE id=200`); err != nil {
			return err
		}
		for i := 0; i < 2; i++ {
			verify, err = tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=200`)
			if err != nil {
				return err
			}
			paired, err := service.bindCollectMaterialPair(ctx, display, verify)
			if err != nil {
				return err
			}
			if !paired {
				t.Fatal("cross-page pair was not bound idempotently")
			}
		}
		row, err := tx.Ctx(ctx).GetOne(`SELECT material_parent_event_id,material_role FROM hg_youban_publish_collect_event WHERE id=200`)
		if err == nil && (row["material_parent_event_id"].Int64() != 1 || row["material_role"].String() != "verify") {
			t.Fatalf("pair not persisted: %v", row)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
