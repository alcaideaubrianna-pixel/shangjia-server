package sys

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/internal/library/cache"
	"hotgo/utility/sourcequeue"
)

// Opt-in disposable loopback PostgreSQL only. This exercises the actual source
// queue and ingest/merge/media SQL boundary. It does not claim to exercise the
// later rule, profile creation, media preparation or Telegram publish stages.
func TestDeliveryFreshAlbumReorderedIngestLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_FRESH_ALBUM_LOCAL_PG_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	// GoFrame caches default Redis clients process-wide. Isolate each run so
	// repeated tests never retain a closed miniredis address or alter other tests.
	if os.Getenv("YOUBAN_FRESH_ALBUM_TEST_CHILD") != "1" {
		childCtx, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		cmd := exec.CommandContext(childCtx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1")
		cmd.Env = append(os.Environ(), "YOUBAN_FRESH_ALBUM_TEST_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated album test failed: %v\n%s", err, output)
		}
		return
	}
	previousDB := gdb.GetConfig("default")
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	redis := miniredis.RunT(t)
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + redis.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cache.SetAdapter(ctx)
	q, err := sourcequeue.New("test:fresh-album:ingest")
	if err != nil {
		t.Fatal(err)
	}
	scope := sourcequeue.Scope{TenantID: 78, SourceID: 351}
	if err := q.Enqueue(ctx, scope, sourcequeue.Realtime, "101", []byte("front")); err != nil {
		t.Fatal(err)
	}
	old, err := q.Claim(ctx, sourcequeue.Realtime, time.Minute)
	if err != nil || old == nil {
		t.Fatalf("old claim: %+v %v", old, err)
	}
	if err := q.Enqueue(ctx, scope, sourcequeue.Fresh, "102", []byte("back")); err != nil {
		t.Fatal(err)
	}
	fresh, err := q.Claim(ctx, sourcequeue.Fresh, time.Minute)
	if err != nil || fresh == nil {
		t.Fatalf("fresh blocked by old inflight: %+v %v", fresh, err)
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, fixture := range []struct {
			name string
			row  interface{}
		}{
			{publishCollectEventTable, entity.YoubanPublishCollectEvent{}},
			{"hg_youban_publish_collect_event_media", entity.YoubanPublishCollectEventMedia{}},
			{"hg_youban_publish_collect_event_log", entity.YoubanPublishCollectEventLog{}},
		} {
			if _, err := tx.Exec(freshAlbumFixtureDDL(fixture.name, fixture.row)); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("CREATE UNIQUE INDEX ON " + publishCollectEventTable + " (source_unique_key)"); err != nil {
			return err
		}
		if _, err := tx.Exec("CREATE UNIQUE INDEX ON hg_youban_publish_collect_event_media (event_id,source_media_key)"); err != nil {
			return err
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		service := &sSysPublish{}
		back := &CollectMessage{TenantId: 78, AccountId: 615, SourceId: 351, SourceType: "account", TgAccountId: 3,
			SourceChatId: "-1003974614787", SourceMessageId: 102, SourceGroupedId: "album-9", SourceUniqueKey: "account:3:source:351:-1003974614787:group:album-9",
			Media: []collectMediaItem{{Type: "photo", FileId: "album-back", FileUniqueId: "back", DebugMetaJson: `{"sourceMessageId":102}`}}, ReceivedAt: gtime.Now()}
		// Fresh gets through while the older realtime claim is still held. The
		// latter supplies the caption and earlier member only after fresh ACK.
		id, err := service.ingestCollectMessage(ctx, back)
		if err != nil {
			return err
		}
		if err := q.Ack(ctx, fresh); err != nil {
			return err
		}
		front := *back
		front.SourceMessageId = 101
		front.RawText = "这是同一相册的正文，前半段媒体虽然晚入库也必须保留正文和完整媒体"
		front.Media = []collectMediaItem{{Type: "photo", FileId: "album-front", FileUniqueId: "front", DebugMetaJson: `{"sourceMessageId":101}`}}
		merged, err := service.ingestCollectMessage(ctx, &front)
		if err != nil {
			return err
		}
		if merged != id {
			t.Fatalf("same album became two events: %d and %d", id, merged)
		}
		if err := q.Ack(ctx, old); err != nil {
			return err
		}
		// Repeated completion/recovery notifications must not duplicate media.
		if replayed, err := service.ingestCollectMessage(ctx, back); err != nil || replayed != id {
			t.Fatalf("album replay failed: id=%d err=%v", replayed, err)
		}
		event, err := tx.GetOne("SELECT * FROM "+publishCollectEventTable+" WHERE id=?", id)
		if err != nil {
			return err
		}
		if event["source_message_id"].Int64() != 101 || event["raw_text"].String() != front.RawText || event["media_count"].Int() != 2 {
			t.Fatalf("reordered album lost caption, earliest ID or media count: %v", event)
		}
		count, err := tx.GetValue("SELECT count(*) FROM " + publishCollectEventTable)
		if err != nil || count.Int() != 1 {
			t.Fatalf("event count=%v err=%v", count, err)
		}
		rows, err := tx.GetAll("SELECT source_media_key FROM hg_youban_publish_collect_event_media WHERE event_id=? ORDER BY source_media_key", id)
		if err != nil || len(rows) != 2 || rows[0]["source_media_key"].String() != "photo:tg:album-back" || rows[1]["source_media_key"].String() != "photo:tg:album-front" {
			t.Fatalf("incomplete or duplicated media: rows=%v err=%v", rows, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Derive only isolated temporary fixture columns from the existing generated
// entities so real DAO field filtering matches production without a prod DSN.
func freshAlbumFixtureDDL(table string, row interface{}) string {
	typ := reflect.TypeOf(row)
	columns := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := field.Tag.Get("orm")
		if name == "" {
			continue
		}
		sqlType := "bigint DEFAULT 0"
		switch {
		case name == "id":
			sqlType = "bigserial PRIMARY KEY"
		case field.Type == reflect.TypeOf((*gtime.Time)(nil)):
			sqlType = "timestamp"
		case field.Type.Kind() == reflect.String:
			sqlType = "text DEFAULT ''"
		case field.Type.Kind() == reflect.Slice:
			sqlType = "bytea"
		}
		columns = append(columns, fmt.Sprintf("%s %s", name, sqlType))
	}
	return "CREATE TEMP TABLE " + table + " (" + strings.Join(columns, ",") + ") ON COMMIT DROP"
}
