package sys

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	publishmodel "hotgo/addons/youban_publish/model"
)

// Executes the actual generated SQL on a fixed local PG, never production.
func TestBotMediaExactLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_STORAGE_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	resetLocalScanTableFields(t, g.DB())
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE TEMP TABLE hg_youban_publish_media(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,md5 text,media_type text,perceptual_hash text,processing_status text,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_media_fingerprint(media_id bigint,tenant_id bigint,account_id bigint,profile_id bigint,phash_bits bit(64),media_type text,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_media_phash_alias_bucket(media_id bigint,tenant_id bigint,account_id bigint,profile_id bigint,hash_value text,media_type text) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_content_profile(id bigint PRIMARY KEY,status int,visibility int,deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP",
			"CREATE INDEX ON hg_youban_publish_media(md5) INCLUDE(profile_id,tenant_id,account_id,media_type) WHERE deleted_at IS NULL AND md5<>''",
			"CREATE INDEX ON hg_youban_publish_media(account_id,md5,tenant_id,profile_id,id) WHERE deleted_at IS NULL AND md5<>''",
			"INSERT INTO hg_youban_publish_media(tenant_id,account_id,profile_id,md5,media_type) SELECT 1,1000+n%501,100000+n,md5(n::text),'image' FROM generate_series(1,200000) n",
			"INSERT INTO hg_content_profile(id,deleted_at) VALUES (10,NULL),(20,NULL),(30,NULL),(40,now()),(50,NULL),(60,NULL)",
			"INSERT INTO hg_youban_publish_profile_state VALUES (10,1,7,NULL),(20,1,7,NULL),(30,2,8,NULL),(40,1,7,NULL),(50,1,7,now())",
			"INSERT INTO hg_youban_publish_media(tenant_id,account_id,profile_id,md5,media_type,deleted_at) VALUES(1,7,10,'byte-exact','image',NULL),(2,7,30,'byte-exact','image',NULL),(1,8,30,'byte-exact','image',NULL),(1,7,40,'byte-exact','image',NULL),(1,7,50,'byte-exact','image',NULL),(1,7,60,'byte-exact','image',NULL),(1,7,10,'byte-exact','image',now())",
			"ANALYZE hg_youban_publish_media",
			"INSERT INTO hg_youban_publish_media_fingerprint VALUES (2,1,7,20,('x00000000000000ff')::bit(64),'image',NULL),(3,2,8,30,('x00000000000000ff')::bit(64),'image',NULL),(4,1,7,40,('x00000000000000ff')::bit(64),'image',NULL),(5,1,7,50,('x00000000000000ff')::bit(64),'image',NULL),(6,1,7,60,('x00000000000000ff')::bit(64),'image',NULL)",
			"INSERT INTO hg_youban_publish_media_phash_alias_bucket VALUES (1,1,7,10,'00000000000000ee','image'),(1,1,7,10,'00000000000000ee','image'),(3,2,8,30,'00000000000000ee','image')",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		scope := &publishmodel.MediaSearchScope{Partitions: []publishmodel.MediaSearchScopePartition{{TenantId: 1, AccountIds: []int64{7}}}}
		for account := int64(1000); account <= 1500; account++ {
			scope.Partitions[0].AccountIds = append(scope.Partitions[0].AccountIds, account)
		}
		hash := goimagehash.NewImageHash(0xff, goimagehash.PHash)
		items := []*mediaFingerprint{{MD5: "byte-exact", PHash: hash}, {PHash: hash}, {PHash: goimagehash.NewImageHash(0xee, goimagehash.PHash)}, {MD5: "missing"}}
		for _, size := range []int{1, 10, 30} {
			batch := make([]*mediaFingerprint, size)
			for i := range batch {
				batch[i] = items[i%len(items)]
			}
			started := time.Now()
			groups, err := botMediaExactCandidates(ctx, batch, scope)
			if err != nil {
				return err
			}
			t.Logf("exact batch=%d elapsed=%s", size, time.Since(started))
			if time.Since(started) > time.Second {
				t.Errorf("exact lookup exceeded 1s at 200k rows and 502 accounts")
			}
			for i := range batch {
				want := [][]int64{{10}, {20}, {10}, nil}[i%len(items)]
				got := rankBotMediaSearchCandidates([][]*botMediaSearchCandidate{groups[i]})
				if !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
					t.Errorf("slot %d got=%v want=%v", i, got, want)
				}
			}
		}
		query, args := botMediaMD5CandidatesSQL([]string{"never-exists"}, scope)
		plan, err := tx.GetAll("EXPLAIN (ANALYZE, FORMAT TEXT) "+query, args...)
		if err != nil {
			return err
		}
		t.Logf("missing fingerprint plan: %s", fmt.Sprint(plan))
		if !strings.Contains(fmt.Sprint(plan), "Index Cond: (md5 = 'never-exists'") {
			t.Error("missing MD5 must use fingerprint-leading lookup")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Temporary tables with the same name can have different fixture schemas.
// GoFrame caches table fields across transactions; keep these tests isolated
// so a previous minimal schema cannot silently strip later write fields.
func resetLocalScanTableFields(t *testing.T, db gdb.DB) {
	t.Helper()
	if err := db.GetCore().ClearTableFieldsAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.GetCore().ClearTableFieldsAll(context.Background()); err != nil {
			t.Error(err)
		}
	})
}
