package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/corona10/goimagehash"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/hibiken/asynq"
	publishmodel "hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMediaPipelineCacheRequiresVisualFingerprint(t *testing.T) {
	for _, kind := range []string{"image", "photo", "video"} {
		source := mediaPipelineSource{MediaType: kind}
		for _, cached := range []*mediaPipelineCacheResult{nil, {MD5: "original"}, {MD5: "original", Processed: true}, {MD5: "original", Processed: true, PerceptualHash: "zzzzzzzzzzzzzzzz"}} {
			if mediaPipelineCacheComplete(source, cached) {
				t.Fatalf("%s accepted incomplete cache %#v", kind, cached)
			}
		}
		complete := &mediaPipelineCacheResult{MD5: "original", Processed: true, PerceptualHash: "1234567890abcdef"}
		if !mediaPipelineCacheComplete(source, complete) {
			t.Fatalf("%s rejected valid cache", kind)
		}
		source.MD5 = "different"
		if mediaPipelineCacheComplete(source, complete) {
			t.Fatalf("%s reused another source", kind)
		}
	}
}

func writeScanStorageImage(t *testing.T, variant int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 128, 96))
	for y := 0; y < 96; y++ {
		for x := 0; x < 128; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x*variant + y), G: uint8(y*3 + variant*x*x), B: uint8(x*y + variant*11), A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "generated.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// Temporary tables in a local transaction; no production connection or Telegram sends.
func TestMediaSearchStorageLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SCAN_STORAGE_PG") != "1" {
		t.Skip("requires local PostgreSQL on 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	resetLocalScanTableFields(t, g.DB())
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
	service := NewSysPublish()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		statements := []string{
			`CREATE TEMP TABLE hg_content_profile(id bigint PRIMARY KEY,status int,visibility int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_note_index(profile_id bigint,tenant_id bigint,account_id bigint,status int,visibility int,deleted_at timestamp) ON COMMIT DROP`,

			`CREATE TEMP TABLE hg_youban_publish_media(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,media_type text,perceptual_hash text,md5 text,processing_status text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_media_fingerprint(media_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,media_type text,phash_bits bit(64),md5 text,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
		}
		for _, table := range []string{publishMediaPHashBucketTable, publishMediaPHashLshTable, publishMediaPHashAliasBucketTable} {
			valueType, extra, key := "integer", "", "media_id,bucket_pos"
			if table == publishMediaPHashBucketTable {
				valueType = "text"
			}
			if table == publishMediaPHashAliasBucketTable {
				extra = "fingerprint_key text,"
				key = "media_id,fingerprint_key,bucket_pos"
			}
			statements = append(statements, fmt.Sprintf(`CREATE TEMP TABLE %s(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,media_id bigint,media_type text,hash_value text,bucket_pos int,bucket_value %s,%s created_at timestamp,updated_at timestamp,UNIQUE(%s)) ON COMMIT DROP`, table, valueType, extra, key))
		}
		for _, sql := range statements {
			if _, err := tx.Ctx(ctx).Exec(sql); err != nil {
				return err
			}
		}

		if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_content_profile VALUES(101,1,1,NULL);
   INSERT INTO hg_youban_publish_profile_state VALUES(101,5,6,NULL);
   INSERT INTO hg_youban_publish_note_index VALUES(101,5,6,1,1,NULL)`); err != nil {
			return err
		}
		t.Run("original_media_and_projection", func(t *testing.T) {
			id, err := service.saveMediaRecordAndIndex(ctx, g.Map{"tenant_id": 5, "account_id": 6, "profile_id": 101, "media_type": "image", "perceptual_hash": "1234567890abcdef", "md5": "0123456789abcdef0123456789abcdef"}, "保存测试媒体失败")
			if err != nil {
				t.Fatal(err)
			}
			row, err := tx.Ctx(ctx).Model("hg_youban_publish_media_fingerprint").Where("media_id", id).One()
			if err != nil {
				t.Fatal(err)
			}
			if row["md5"].String() != "0123456789abcdef0123456789abcdef" {
				t.Fatalf("lost original MD5: %v", row)
			}
			if _, err = tx.Ctx(ctx).Exec("DELETE FROM "+publishMediaPHashLshTable+" WHERE media_id=?", id); err != nil {
				t.Fatal(err)
			}
			if err = service.syncMediaPHashBucketByMediaId(ctx, id); err != nil {
				t.Fatal(err)
			}
			count, err := tx.Ctx(ctx).Model(publishMediaPHashLshTable).Where("media_id", id).Count()
			if err != nil || count != 4 {
				t.Fatalf("repair count=%d err=%v", count, err)
			}
		})
		t.Run("insert_rolls_back_if_index_fails", func(t *testing.T) {
			if _, err := tx.Ctx(ctx).Exec("ALTER TABLE " + publishMediaPHashLshTable + " ADD CONSTRAINT reject_profile CHECK(profile_id<>999)"); err != nil {
				t.Fatal(err)
			}
			_, err := service.saveMediaRecordAndIndex(ctx, g.Map{"tenant_id": 5, "account_id": 6, "profile_id": 999, "media_type": "image", "perceptual_hash": "1234567890abcdef"}, "保存测试媒体失败")
			if err == nil {
				t.Fatal("expected index error")
			}
			count, err := tx.Ctx(ctx).Model(publishMediaTable).Where("profile_id", 999).Count()
			if err != nil || count != 0 {
				t.Fatalf("partial media persisted count=%d err=%v", count, err)
			}
		})
		t.Run("ready_retry_does_not_skip_failed_index", func(t *testing.T) {
			if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_media(id,tenant_id,account_id,profile_id,media_type,perceptual_hash,md5,processing_status) VALUES(600,5,6,999,'image','1234567890abcdef','original-md5','ready')`); err != nil {
				t.Fatal(err)
			}
			err := service.handleMediaProcessTask(ctx, asynq.NewTask("local", []byte(`{"mediaId":600}`)))
			if err == nil || !strings.Contains(err.Error(), "写入媒体LSH索引失败") {
				t.Fatalf("ready retry must try failed index, err=%v", err)
			}
			// Do not leave an intentionally invalid source in later negative searches.
			if _, err := tx.Ctx(ctx).Exec("DELETE FROM hg_youban_publish_media WHERE id=600"); err != nil {
				t.Fatal(err)
			}
		})
		t.Run("editing_media_rolls_back_and_removes_stale_index", func(t *testing.T) {
			id, err := service.saveMediaRecordAndIndex(ctx, g.Map{"tenant_id": 5, "account_id": 6, "profile_id": 101, "media_type": "image", "perceptual_hash": "1234567890abcdef", "md5": "original-md5"}, "保存测试媒体失败")
			if err != nil {
				t.Fatal(err)
			}
			_, err = service.saveMediaDataAndIndex(ctx, id, g.Map{"profile_id": 999, "perceptual_hash": "abcdef1234567890"}, "更新测试媒体失败")
			if err == nil {
				t.Fatal("expected index failure")
			}
			original, err := tx.Ctx(ctx).Model(publishMediaTable).Where("id", id).One()
			if err != nil {
				t.Fatal(err)
			}
			if original["profile_id"].Int64() != 101 || original["perceptual_hash"].String() != "1234567890abcdef" {
				t.Fatal("failed edit was not rolled back")
			}
			if _, err = service.saveMediaDataAndIndex(ctx, id, g.Map{"perceptual_hash": ""}, "更新测试媒体失败"); err != nil {
				t.Fatal(err)
			}
			for _, table := range []string{publishMediaPHashBucketTable, publishMediaPHashLshTable, "hg_youban_publish_media_fingerprint"} {
				n, err := tx.Ctx(ctx).Model(table).Where("media_id", id).Count()
				if err != nil || n != 0 {
					t.Fatalf("stale index remained table=%s rows=%d err=%v", table, n, err)
				}
			}
		})

		t.Run("generated_versions_and_idempotent_reuse", func(t *testing.T) {
			if _, err := tx.Ctx(ctx).Exec("INSERT INTO " + publishMediaTable + "(id,tenant_id,account_id,profile_id,media_type,perceptual_hash) VALUES(500,5,6,101,'image','1234567890abcdef')"); err != nil {
				t.Fatal(err)
			}
			media := &telegramMediaItem{Id: 500, MediaType: "image", TenantId: 5, AccountId: 552, JobId: 42}
			first, second := writeScanStorageImage(t, 1), writeScanStorageImage(t, 7)
			a, err := telegramAntiScanFileHash(first)
			if err != nil {
				t.Fatal(err)
			}
			b, err := telegramAntiScanFileHash(second)
			if err != nil {
				t.Fatal(err)
			}
			if a.PHash == b.PHash {
				t.Fatal("fixtures need different hashes")
			}
			for _, path := range []string{first, second, first} {
				if err := service.indexTelegramGeneratedMedia(ctx, media, path, "image"); err != nil {
					t.Fatal(err)
				}
			}
			rows, err := tx.Ctx(ctx).Model(publishMediaPHashAliasBucketTable).Where("media_id", 500).All()
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 8 {
				t.Fatalf("want two complete versions, got %d", len(rows))
			}
			for _, row := range rows {
				if row["account_id"].Int64() != 6 || row["tenant_id"].Int64() != 5 || row["profile_id"].Int64() != 101 {
					t.Fatalf("wrong owner: %v", row)
				}
			}
			scope := &publishmodel.MediaSearchScope{AccountIds: []int64{6}, Partitions: []publishmodel.MediaSearchScopePartition{{TenantId: 5, AccountIds: []int64{6}}}}
			input := &sysin.ProfileImageSearchInp{Threshold: 12}
			if err := markMediaPHashLshReady(ctx); err != nil {
				t.Fatal(err)
			}
			for _, hash := range []uint64{a.PHash, b.PHash} {
				for round := 0; round < 2; round++ {
					started := time.Now()
					found, err := service.botMediaSearchFingerprintCandidates(ctx, &mediaFingerprint{PHash: goimagehash.NewImageHash(hash, goimagehash.PHash)}, input, scope)
					if err != nil || len(found) != 1 || found[0].ProfileId != 101 || found[0].Distance != 0 {
						t.Fatalf("generated exact search round=%d found=%+v err=%v", round, found, err)
					}
					t.Logf("generated version exact search round=%d elapsed=%s", round, time.Since(started))
				}
				// A changed/compressed input must be recalled through the alias LSH too.
				changed := goimagehash.NewImageHash(hash^0x0001000100010001, goimagehash.PHash)
				found, err := service.botMediaSearchFingerprintCandidates(ctx, &mediaFingerprint{PHash: changed}, input, scope)
				if err != nil || len(found) != 1 || found[0].ProfileId != 101 || found[0].Distance != 4 {
					t.Fatalf("alias approximate search found=%+v err=%v", found, err)
				}
			}
			unrelated, negativeErr := service.botMediaSearchFingerprintCandidates(ctx, &mediaFingerprint{PHash: goimagehash.NewImageHash(0, goimagehash.PHash)}, input, scope)
			if negativeErr != nil || len(unrelated) != 0 {
				t.Fatalf("unrelated image returned results=%+v err=%v", unrelated, negativeErr)
			}

			forbidden := &publishmodel.MediaSearchScope{Partitions: []publishmodel.MediaSearchScopePartition{{TenantId: 9, AccountIds: []int64{6}}}}
			found, err := service.botMediaSearchFingerprintCandidates(ctx, &mediaFingerprint{PHash: goimagehash.NewImageHash(a.PHash, goimagehash.PHash)}, input, forbidden)
			if err != nil || len(found) != 0 {
				t.Fatalf("cross-tenant search leaked found=%+v err=%v", found, err)
			}

		})
		t.Run("generation_failure_falls_back_to_source_upload", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "document.jpg")
			if err := os.WriteFile(path, encodeBackgroundReplacementTestImage(t, color.White), 0600); err != nil {
				t.Fatal(err)
			}
			cleaned := false
			// The fallback still indexes the final upload. Give it a real,
			// permission-owned source instead of bypassing that invariant.
			id, err := service.saveMediaRecordAndIndex(ctx, g.Map{"tenant_id": 5, "account_id": 6, "profile_id": 101, "media_type": "image"}, "保存回退测试媒体失败")
			if err != nil {
				t.Fatal(err)
			}
			media := &telegramMediaItem{Id: id, MediaType: "image", AntiScanMode: "background_replace", AntiScanBackgroundURL: "unused"}
			out, cleanup, err := service.prepareTelegramAntiScanUploadFile(ctx, media, path, func() { cleaned = true }, "image")
			if err != nil || out == "" || cleanup == nil || cleaned {
				t.Fatalf("out=%q err=%v cleaned=%v", out, err, cleaned)
			}
			count, err := tx.Ctx(ctx).Model(publishMediaPHashAliasBucketTable).Where("media_id", id).Count()
			if err != nil || count != 4 {
				t.Fatalf("fallback upload not searchable: buckets=%d err=%v", count, err)
			}
			cleanup()
		})
		t.Run("fallback_rejects_missing_original", func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "document.jpg")
			if err := os.WriteFile(path, encodeBackgroundReplacementTestImage(t, color.White), 0600); err != nil {
				t.Fatal(err)
			}
			cleaned := false
			media := &telegramMediaItem{Id: 999999, MediaType: "image", AntiScanMode: "background_replace", AntiScanBackgroundURL: "unused"}
			out, cleanup, err := service.prepareTelegramAntiScanUploadFile(ctx, media, path, func() { cleaned = true }, "image")
			if err == nil || out != "" || cleanup != nil || !cleaned {
				t.Fatalf("missing source was accepted: out=%q err=%v cleaned=%v", out, err, cleaned)
			}
		})
		t.Run("alias_bucket_failure_is_atomic", func(t *testing.T) {
			if _, err := tx.Ctx(ctx).Exec("ALTER TABLE " + publishMediaPHashAliasBucketTable + " ADD CONSTRAINT reject_second CHECK(media_id<>501 OR bucket_pos<>2)"); err != nil {
				t.Fatal(err)
			}
			err := service.saveAntiScanSearchFingerprint(ctx, telegramJobRecord{TenantId: 5, AccountId: 6, ProfileId: 101}, &telegramSentMessage{MediaId: 501, ProtectedHashKey: "atomic", ProtectedPHash: 0x123456789abcdef0})
			if err == nil {
				t.Fatal("expected bucket write failure")
			}
			count, err := tx.Ctx(ctx).Model(publishMediaPHashAliasBucketTable).Where("media_id", 501).Count()
			if err != nil || count != 0 {
				t.Fatalf("partial version count=%d err=%v", count, err)
			}
		})
		t.Run("historical_images_local_database", func(t *testing.T) {
			root := os.Getenv("YOUBAN_SCAN_FIXTURE_ROOT")
			if root == "" {
				t.Skip("set private historical fixtures root")
			}
			raw, err := os.ReadFile(filepath.Join(root, "search-snapshot.json"))
			if err != nil {
				t.Fatal(err)
			}
			var snapshot struct {
				Media   []g.Map `json:"media"`
				Aliases []g.Map `json:"aliases"`
			}
			if err = json.Unmarshal(raw, &snapshot); err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Media) == 0 || len(snapshot.Aliases) == 0 {
				t.Fatal("empty historical snapshot")
			}
			owners := map[int64]bool{}
			for _, row := range snapshot.Media {
				id := gvar.New(row["id"]).Int64()
				profile := gvar.New(row["profile_id"]).Int64()
				if _, err := tx.Ctx(ctx).Model(publishMediaTable).Data(row).Insert(); err != nil {
					t.Fatal(err)
				}
				if err := service.syncMediaPHashBucketByMediaId(ctx, id); err != nil {
					t.Fatal(err)
				}
				if !owners[profile] {
					owners[profile] = true
					if _, err := tx.Ctx(ctx).Exec("INSERT INTO hg_content_profile VALUES(?,1,1,NULL)", profile); err != nil {
						t.Fatal(err)
					}
					if _, err := tx.Ctx(ctx).Exec("INSERT INTO hg_youban_publish_profile_state VALUES(?,?,?,NULL)", profile, row["tenant_id"], row["account_id"]); err != nil {
						t.Fatal(err)
					}
					if _, err := tx.Ctx(ctx).Exec("INSERT INTO hg_youban_publish_note_index VALUES(?,?,?,1,1,NULL)", profile, row["tenant_id"], row["account_id"]); err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, row := range snapshot.Aliases {
				if _, err := tx.Ctx(ctx).Model(publishMediaPHashAliasBucketTable).Data(row).Insert(); err != nil {
					t.Fatal(err)
				}
			}
			server := httptest.NewServer(http.FileServer(http.Dir(root)))
			defer server.Close()
			cases := []struct {
				name, pattern string
				want          int64
				count         int
			}{
				{"EEKFQFFA", "other-all/ee*.jpg", 676231, 6},
				{"JNAAUKWG", "other-all/jn*.jpg", 302321, 2},
				{"GVJFGVCU", "gvj-all/*.jpg", 676231, 6},
			}
			singles, err := filepath.Glob(filepath.Join(root, "sent-tg", "*.jpg"))
			if err != nil || len(singles) != 18 {
				t.Fatalf("expected 18 sent fixtures, got %d err=%v", len(singles), err)
			}
			for _, path := range singles {
				cases = append(cases, struct {
					name, pattern string
					want          int64
					count         int
				}{filepath.Base(path), "sent-tg/" + filepath.Base(path), 676231, 1})
			}
			scope := &publishmodel.MediaSearchScope{AccountIds: []int64{6}, Partitions: []publishmodel.MediaSearchScopePartition{{TenantId: 5, AccountIds: []int64{6}}}}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					cache.Instance().SetAdapter(gcache.NewAdapterMemory())
					g.DB().GetCache().SetAdapter(gcache.NewAdapterMemory())
					if err := markMediaPHashLshReady(ctx); err != nil {
						t.Fatal(err)
					}
					paths, err := filepath.Glob(filepath.Join(root, test.pattern))
					if err != nil || len(paths) != test.count {
						t.Fatalf("fixture count=%d expected=%d err=%v", len(paths), test.count, err)
					}
					for round := 0; round < 2; round++ {
						started := time.Now()
						groups := [][]*botMediaSearchCandidate{}
						for _, path := range paths {
							rel, _ := filepath.Rel(root, path)
							fingerprint, _, err := cachedTelegramImageFingerprint(ctx, "local-history-"+test.name+filepath.Base(path), server.URL+"/"+filepath.ToSlash(rel))
							if err != nil {
								t.Fatal(err)
							}
							found, err := service.botMediaSearchFingerprintCandidates(ctx, fingerprint, &sysin.ProfileImageSearchInp{Threshold: 12}, scope)
							if err != nil {
								t.Fatal(err)
							}
							groups = append(groups, found)
						}
						ids := rankBotMediaSearchCandidates(groups)
						if len(ids) != 1 || ids[0] != test.want {
							t.Fatalf("wrong historical result ids=%v want=%d", ids, test.want)
						}
						elapsed := time.Since(started)
						budget := 3 * time.Second
						if round == 1 {
							budget = 500 * time.Millisecond
						}
						if elapsed > budget {
							t.Fatalf("historical local replay exceeded budget: %s", elapsed)
						}
						t.Logf("historical local download/hash/search round=%d elapsed=%s profile=%d", round, elapsed, ids[0])
					}
				})
			}
		})

		t.Run("lsh_distance_filter_preserves_threshold_and_scope", func(t *testing.T) {
			fixtures := []struct {
				id, tenant int64
				hash       string
			}{
				{990001, 5, "000f000f000f0000"}, // distance 12: included
				{990002, 5, "000f000f000f0001"}, // distance 13: excluded, despite matching LSH buckets
				{990003, 9, "000f000f000f0000"}, // same hash, another tenant
			}
			for _, item := range fixtures {
				if _, err := tx.Exec(`INSERT INTO hg_youban_publish_media(id,tenant_id,account_id,profile_id,media_type,perceptual_hash) VALUES(?,?,6,101,'image',?)`, item.id, item.tenant, item.hash); err != nil {
					t.Fatal(err)
				}
				for _, cell := range mediaPHashLshBucketValues(item.hash) {
					if _, err := tx.Exec(`INSERT INTO hg_youban_publish_media_phash_lsh(tenant_id,account_id,profile_id,media_id,media_type,hash_value,bucket_pos,bucket_value) VALUES(?,6,101,?,'image',?,?,?)`, item.tenant, item.id, item.hash, cell.Pos, cell.Value); err != nil {
						t.Fatal(err)
					}
				}
			}
			query, args, err := mediaPHashLshCandidateSQL("0000000000000000", 12, []mediaPHashBucketScopePart{{TenantId: 5, AccountIds: []int64{6}}}, nil, "image", 0)
			if err != nil {
				t.Fatal(err)
			}
			var found []mediaPHashBucketCandidateRow
			if err := tx.Raw(query, args...).Scan(&found); err != nil {
				t.Fatal(err)
			}
			hit := false
			for _, item := range found {
				if item.MediaId == 990002 || item.MediaId == 990003 {
					t.Fatalf("distance/scope violation: %+v", item)
				}
				if item.MediaId == 990001 {
					hit = true
				}
			}
			if !hit {
				t.Fatal("lost distance-12 candidate")
			}
			scopedSQL, scopedArgs, err := mediaPHashLshCandidateSQLWithStrategy("0000000000000000", 12, []mediaPHashBucketScopePart{{TenantId: 5, AccountIds: []int64{6}}}, nil, "image", 0, true)
			if err != nil {
				t.Fatal(err)
			}
			var scoped []mediaPHashBucketCandidateRow
			if err := tx.Raw(scopedSQL, scopedArgs...).Scan(&scoped); err != nil {
				t.Fatal(err)
			}
			identity := func(rows []mediaPHashBucketCandidateRow) map[string]bool {
				ids := make(map[string]bool)
				for _, row := range rows {
					ids[fmt.Sprintf("%d:%d:%s", row.MediaId, row.ProfileId, row.HashValue)] = true
				}
				return ids
			}
			if !reflect.DeepEqual(identity(found), identity(scoped)) {
				t.Fatalf("scoped scan differs from LSH: lsh=%v scoped=%v", identity(found), identity(scoped))
			}
		})

		t.Run("bot_batch_chunks_cache_and_ranking", func(t *testing.T) {
			if err := markMediaPHashLshReady(ctx); err != nil {
				t.Fatal(err)
			}
			scope := &publishmodel.MediaSearchScope{Partitions: []publishmodel.MediaSearchScopePartition{{TenantId: 5, AccountIds: []int64{6}}}}
			input := &sysin.ProfileImageSearchInp{Threshold: 12}
			for _, size := range []int{1, 10, 30} {
				fingerprints := make([]*mediaFingerprint, size)
				for i := range fingerprints {
					fingerprints[i] = &mediaFingerprint{PHash: goimagehash.NewImageHash(uint64(i), goimagehash.PHash)}
				}
				cache.Instance().SetAdapter(gcache.NewAdapterMemory())
				if err := markMediaPHashLshReady(ctx); err != nil {
					t.Fatal(err)
				}
				batch, err := service.botMediaApproximateBatch(ctx, fingerprints, input, scope)
				if err != nil {
					t.Fatal(err)
				}
				cached, err := service.botMediaApproximateBatch(ctx, fingerprints, input, scope)
				if err != nil || !reflect.DeepEqual(batch, cached) {
					t.Fatalf("size=%d cache differs err=%v", size, err)
				}
				cache.Instance().SetAdapter(gcache.NewAdapterMemory())
				if err := markMediaPHashLshReady(ctx); err != nil {
					t.Fatal(err)
				}
				old := make([][]*botMediaSearchCandidate, size)
				for i, f := range fingerprints {
					old[i], err = service.botMediaFingerprintCandidates(ctx, f, input, scope, true)
					if err != nil {
						t.Fatal(err)
					}
					// Empty slices and nil both represent a confirmed no-match.
					if len(old[i]) != len(batch[i]) || (len(old[i]) > 0 && !reflect.DeepEqual(old[i], batch[i])) {
						t.Fatalf("size=%d input=%d candidates differ", size, i)
					}
				}
				if !reflect.DeepEqual(rankBotMediaSearchCandidates(old), rankBotMediaSearchCandidates(batch)) {
					t.Fatalf("size=%d ranking differs", size)
				}
			}
			// A changed index version must not reuse another album's old cache.
			if _, err := tx.Ctx(ctx).Exec("UPDATE hg_youban_publish_profile_state SET deleted_at=now() WHERE tenant_id=5 AND account_id=6"); err != nil {
				t.Fatal(err)
			}
			if err := bumpMediaPHashBucketVersion(ctx, 5, 6); err != nil {
				t.Fatal(err)
			}
			found, err := service.botMediaApproximateBatch(ctx, []*mediaFingerprint{{PHash: goimagehash.NewImageHash(0, goimagehash.PHash)}, {PHash: goimagehash.NewImageHash(1, goimagehash.PHash)}}, input, scope)
			if err != nil || len(rankBotMediaSearchCandidates(found)) != 0 {
				t.Fatalf("stale deleted profile returned: %+v err=%v", found, err)
			}
		})

		t.Run("lsh_scope_strategy_uses_statistics", func(t *testing.T) {
			if address := os.Getenv("YOUBAN_SCAN_TEST_REDIS_ADDRESS"); address != "" {
				client, err := gredis.New(&gredis.Config{Address: address})
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close(ctx)
				previous := cache.Instance().GetAdapter()
				cache.Instance().SetAdapter(gcache.NewAdapterRedis(client))
				defer cache.Instance().SetAdapter(previous)
			}
			if _, err := tx.Exec(`INSERT INTO hg_youban_publish_media_phash_lsh(tenant_id,account_id,profile_id,media_id,media_type,hash_value,bucket_pos,bucket_value)
SELECT 77,6,101,10000000+n,'image','ffffffffffffffff',1,65535 FROM generate_series(1,?) n`, 2*mediaPHashLshScopeScanLimit); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec("ANALYZE hg_youban_publish_media_phash_lsh"); err != nil {
				t.Fatal(err)
			}
			for tenant, want := range map[int64]bool{77: false, 78: true} {
				got, err := mediaPHashLshScanSmallScope(ctx, []mediaPHashBucketScopePart{{TenantId: tenant, AccountIds: []int64{6}}}, nil, "image", 0)
				if err != nil || got != want {
					t.Fatalf("tenant=%d scopeScan=%v want=%v err=%v", tenant, got, want, err)
				}
			}
		})

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMediaProcessingReadyRequiresPHash(t *testing.T) {
	for _, kind := range []string{"image", "video", "photo"} {
		row := gdb.Record{"md5": gvar.New("original"), "media_type": gvar.New(kind), "perceptual_hash": gvar.New("")}
		if mediaProcessingFingerprintsComplete(row) {
			t.Fatalf("%s accepted missing pHash", kind)
		}
		row["perceptual_hash"] = gvar.New("1234567890abcdef")
		if !mediaProcessingFingerprintsComplete(row) {
			t.Fatalf("%s rejected complete pHash", kind)
		}
	}
}
