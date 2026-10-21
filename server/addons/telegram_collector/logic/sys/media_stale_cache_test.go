package sys

import (
	"context"
	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/cache"
	"os"
	"testing"
	"time"
)

func TestMediaStaleProcessingCacheLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires local PostgreSQL")
	}
	previous := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", previous)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	redis := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: redis.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	if !cache.Initialized() {
		cache.SetAdapter(context.Background())
	}
	cache.Instance().SetAdapter(gcache.NewAdapterRedis(client))
	defer cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	err = g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("CREATE TEMP TABLE hg_tg_collector_media(id bigint,tenant_id bigint,fingerprint text,pipeline_version text,status text,file_url text,updated_at timestamp) ON COMMIT DROP"); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO hg_tg_collector_media VALUES(1,0,'stale-regression','v1','ready','https://example.test/a.jpg',now())"); err != nil {
			return err
		}
		key := consts.MediaCacheKey("stale-regression")
		stale, _ := json.Marshal(sysin.MediaCacheEntry{Status: sysin.MediaStatusProcessing})
		if err := cache.Instance().Set(ctx, key, string(stale), 720*time.Hour); err != nil {
			return err
		}
		entry, ready, err := NewCollector().MediaCache(ctx, "stale-regression")
		if err != nil {
			return err
		}
		if !ready || entry == nil || entry.FileURL != "https://example.test/a.jpg" {
			t.Fatalf("stale cache blocked ready: %+v %v", entry, ready)
		}
		val, err := cache.Instance().Get(ctx, key)
		if err != nil {
			return err
		}
		var refreshed sysin.MediaCacheEntry
		if err := json.Unmarshal([]byte(val.String()), &refreshed); err != nil {
			return err
		}
		if refreshed.Status != sysin.MediaStatusReady {
			t.Fatal("cache not repaired")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
