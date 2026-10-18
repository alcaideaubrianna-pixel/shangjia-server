package fix

import (
	"context"
	"fmt"
	"os"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"
	_ "hotgo/addons/youban_publish/logic/sys"
	"hotgo/internal/library/cache"
)

func TestMediaSearchRepairOptions(t *testing.T) {
	options, err := parseMediaSearchRepairOptions(nil)
	if err != nil || options.apply || options.limit != 100 {
		t.Fatalf("unsafe defaults %+v %v", options, err)
	}
	for _, args := range []map[string]string{{"apply": "true"}, {"limit": "0"}, {"limit": "1001"}, {"limit": "abc"}, {"afterId": "-1"}, {"tenantId": "other"}, {"profileId": "-2"}} {
		if _, err := parseMediaSearchRepairOptions(args); err == nil {
			t.Fatalf("accepted invalid options %v", args)
		}
	}
	for hash, want := range map[string]bool{"1234567890abcdef": true, " 1234567890ABCDEF ": true, "zzzzzzzzzzzzzzzz": false, "": false} {
		if got := validMediaSearchRepairHash(hash); got != want {
			t.Fatalf("hash %q valid=%v", hash, got)
		}
	}
}

func TestMediaSearchIndexRepairLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_STORAGE_PG") != "1" {
		t.Skip("requires local PostgreSQL on 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.Background()
	if !cache.Initialized() {
		// Update a config value through the adapter's synchronized setter rather
		// than replacing the global adapter while file-cache prune goroutines run.
		adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
		if !ok {
			t.Fatal("local storage test requires file configuration adapter")
		}
		original := g.Cfg().MustGet(ctx, "cache.adapter").Val()
		if err := adapter.Set("cache.adapter", "memory"); err != nil {
			t.Fatal(err)
		}
		cache.SetAdapter(ctx)
		if err := adapter.Set("cache.adapter", original); err != nil {
			t.Fatal(err)
		}
	}
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previous) })
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		statements := []string{
			`CREATE TEMP TABLE hg_youban_publish_media(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,media_type text,perceptual_hash text,md5 text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_media_fingerprint(media_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,media_type text,phash_bits bit(64),md5 text,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
		}
		for _, table := range []string{youbanPublishMediaPHashBucketTable, youbanPublishMediaPHashLshTable} {
			valueType := "integer"
			if table == youbanPublishMediaPHashBucketTable {
				valueType = "text"
			}
			statements = append(statements, fmt.Sprintf(`CREATE TEMP TABLE %s(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,media_id bigint,media_type text,hash_value text,bucket_pos int,bucket_value %s,created_at timestamp,updated_at timestamp,UNIQUE(media_id,bucket_pos)) ON COMMIT DROP`, table, valueType))
		}
		for _, sql := range statements {
			if _, err := tx.Ctx(ctx).Exec(sql); err != nil {
				return err
			}
		}
		if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_media(id,tenant_id,account_id,profile_id,media_type,perceptual_hash,md5) VALUES
   (10,5,6,101,'image','1234567890abcdef','original-md5'),
   (11,5,6,101,'image','1234567890ABCDEF','second-md5'),
   (12,9,6,101,'image','1234567890abcdef','other-tenant'),
   (13,5,6,101,'image','','missing'),
   (14,5,6,101,'image','zzzzzzzzzzzzzzzz','invalid')`); err != nil {
			return err
		}
		args := map[string]string{"tenantId": "5", "profileId": "101", "limit": "1"}
		if err := RepairYoubanPublishMediaSearchIndexes(ctx, args); err != nil {
			return err
		}
		n, err := tx.Ctx(ctx).Model(youbanPublishMediaPHashLshTable).Count()
		if err != nil {
			return err
		}
		if n != 0 {
			t.Fatal("dry run wrote indexes")
		}
		args["apply"] = "1"
		if err := RepairYoubanPublishMediaSearchIndexes(ctx, args); err != nil {
			return err
		}
		n, err = tx.Ctx(ctx).Model(youbanPublishMediaPHashLshTable).Count()
		if err != nil {
			return err
		}
		if n != 4 {
			t.Fatalf("bounded first batch=%d", n)
		}
		// Keep all bucket rows but corrupt their contents. The audit must detect it.
		if _, err := tx.Ctx(ctx).Exec("UPDATE " + youbanPublishMediaPHashLshTable + " SET bucket_value=0 WHERE media_id=10"); err != nil {
			return err
		}
		if err := RepairYoubanPublishMediaSearchIndexes(ctx, args); err != nil {
			return err
		}
		valid, err := g.DB().GetValue(ctx, mediaSearchIndexCompleteSQL, 10)
		if err != nil {
			return err
		}
		if !valid.Bool() {
			t.Fatal("corrupt index not repaired")
		}
		args["afterId"] = "10"
		args["limit"] = "100"
		if err := RepairYoubanPublishMediaSearchIndexes(ctx, args); err != nil {
			return err
		}
		for id, want := range map[int64]int{10: 4, 11: 4, 12: 0, 13: 0, 14: 0} {
			n, err := tx.Ctx(ctx).Model(youbanPublishMediaPHashLshTable).Where("media_id", id).Count()
			if err != nil {
				return err
			}
			if n != want {
				t.Fatalf("media=%d count=%d want=%d", id, n, want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
