package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gogf/gf/v2/database/gdb"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/internal/library/cache"
)

type duplicateCustomerFixture struct {
	URL       string              `json:"url"`
	ProfileID int64               `json:"profileId"`
	Images    []duplicateImageRow `json:"images"`
}

func loadDuplicateCustomerFixtures(t *testing.T) []duplicateCustomerFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/duplicate_kkbaoyang_20261023.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []duplicateCustomerFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 3 {
		t.Fatalf("expected three customer links, got %d", len(fixtures))
	}
	return fixtures
}

func TestDuplicateCustomerAnyImageMatch(t *testing.T) {
	fixtures := loadDuplicateCustomerFixtures(t)
	for i := range fixtures {
		for j := i + 1; j < len(fixtures); j++ {
			t.Run(fmt.Sprintf("%d_%d", i, j), func(t *testing.T) {
				if !duplicateScanImagesMatch(fixtures[i].Images, fixtures[j].Images) {
					t.Fatalf("customer duplicates missed: %s / %s", fixtures[i].URL, fixtures[j].URL)
				}
				left, _ := duplicateProfileSignature("", fixtures[i].Images)
				right, _ := duplicateProfileSignature("", fixtures[j].Images)
				if left == right {
					t.Fatal("fixture does not reproduce previous whole-album mismatch")
				}
			})
		}
	}
	// Boundary checks exercise the actual final similarity predicate.
	if !duplicateScanImagesMatch([]duplicateImageRow{{PerceptualHash: "0000000000000000"}}, []duplicateImageRow{{PerceptualHash: "0000000000000fff"}}) {
		t.Fatal("distance 12 should match")
	}
	if duplicateScanImagesMatch([]duplicateImageRow{{PerceptualHash: "0000000000000000"}}, []duplicateImageRow{{PerceptualHash: "0000000000001fff"}}) {
		t.Fatal("distance 13 should not match")
	}
	if duplicateScanImagesMatch(fixtures[0].Images, []duplicateImageRow{{Md5: "different", PerceptualHash: "0000000000000000"}}) {
		t.Fatal("unrelated image matched")
	}
	if !duplicateScanImagesMatch([]duplicateImageRow{{Md5: "same"}, {}}, []duplicateImageRow{{Md5: "same"}}) {
		t.Fatal("missing hash on another image blocked valid match")
	}
	if duplicateScanImagesMatch([]duplicateImageRow{{PerceptualHash: "invalid"}}, []duplicateImageRow{{PerceptualHash: "invalid"}}) {
		t.Fatal("invalid hash accepted")
	}
}

// Redis state runs in a fresh process to avoid replacing another test's default
// connection. This test uses only a local disposable Redis, never production.
func TestDuplicateSimilarIndexAcrossPages(t *testing.T) {
	if os.Getenv("YOUBAN_DUPLICATE_TEST_REDIS") == "" {
		redis := miniredis.RunT(t)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDuplicateSimilarIndexAcrossPages$", "-test.count=1")
		cmd.Env = append(os.Environ(), "YOUBAN_DUPLICATE_TEST_REDIS="+redis.Addr())
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated Redis regression: %v\n%s", err, output)
		}
		return
	}
	addr := os.Getenv("YOUBAN_DUPLICATE_TEST_REDIS")
	gredis.SetConfig(&gredis.Config{Address: addr})
	adapter := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
	if err := adapter.Set("redis.default.address", addr); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Set("redis.default.pass", ""); err != nil {
		t.Fatal(err)
	}
	fixtures := loadDuplicateCustomerFixtures(t)
	if os.Getenv("YOUBAN_LOCAL_DUPLICATE_PG") == "1" {
		runDuplicateCustomerSQLRegression(t, fixtures)
	}
	orders := [][]int{{0, 1, 2}, {2, 1, 0}, {1, 0, 2}}
	for n, order := range orders {
		token := fmt.Sprintf("customer-%d", n)
		for _, index := range order {
			// A new cache for every profile exercises cross-page Redis reloads.
			w := newDuplicateScanWorkCache(context.Background(), token)
			id := int64(index + 1) // Fixture-local identity; third link has no database ID.
			if err := w.appendSimilarImages(id, id, fixtures[index].Images); err != nil {
				t.Fatal(err)
			}
			// Retry before save must not duplicate a member or discard dirty buckets.
			if err := w.appendSimilarImages(id, id, fixtures[index].Images); err != nil {
				t.Fatal(err)
			}
			if err := w.save(); err != nil {
				t.Fatal(err)
			}
		}
		pairs := map[string]bool{}
		err := rangeDuplicateScanWorkGroups(context.Background(), token, func(key string, group *duplicateScanWorkGroup) error {
			if !strings.HasPrefix(key, "phash:") {
				return nil
			}
			for _, id := range group.MemberIds[1:] {
				if !duplicateScanImagesMatch(fixtures[group.KeepProfileId-1].Images, fixtures[id-1].Images) {
					t.Fatalf("transitive false match: %+v", group)
				}
				pairs[fmt.Sprintf("%d:%d", group.KeepProfileId, id)] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, pair := range []string{"2:1", "3:1", "3:2"} {
			if !pairs[pair] {
				t.Fatalf("order %v missed %s: %v", order, pair, pairs)
			}
		}
	}
	// Non-transitive A~B~C: preserve both direct matches, never invent A~C.
	rows := [][]duplicateImageRow{{{Md5: "ab"}}, {{Md5: "ab"}, {Md5: "bc"}}, {{Md5: "bc"}}}
	for i := range rows {
		w := newDuplicateScanWorkCache(context.Background(), "chain")
		if err := w.appendSimilarImages(int64(i+1), int64(i+1), rows[i]); err != nil {
			t.Fatal(err)
		}
		if err := w.save(); err != nil {
			t.Fatal(err)
		}
	}
	if err := rangeDuplicateScanWorkGroups(context.Background(), "chain", func(key string, group *duplicateScanWorkGroup) error {
		if !strings.HasPrefix(key, "phash:") {
			return nil
		}
		for _, id := range group.MemberIds[1:] {
			if !duplicateScanImagesMatch(rows[group.KeepProfileId-1], rows[id-1]) {
				t.Fatalf("transitive false match: %+v", group)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Optional real SQL replay of the production scan's page ingestion. Only a
// disposable schema at localhost:35432 is used; source rows must remain intact.
func runDuplicateCustomerSQLRegression(t *testing.T, fixtures []duplicateCustomerFixture) {
	t.Helper()
	ctx := context.Background()
	schema := fmt.Sprintf("account_lifecycle_%d", time.Now().UnixNano())
	admin, err := gdb.New(accountLifecycleConfig("public"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	gdb.SetConfigGroup("default", gdb.ConfigGroup{accountLifecycleConfig(schema)})
	db := g.DB()
	for _, sql := range []string{
		"CREATE TABLE hg_content_profile(id bigint PRIMARY KEY,source_note_uuid text,profile_no text,title text,created_at timestamp,deleted_at timestamp)",
		"CREATE TABLE hg_youban_publish_media(id bigserial PRIMARY KEY,profile_id bigint,md5 text,perceptual_hash text,media_type text,purpose text,sort_index int,deleted_at timestamp,file_url text,storage_path text,edited_file_url text,edited_storage_path text)",
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	for i, fixture := range fixtures {
		if _, err := db.Exec(ctx, "INSERT INTO hg_content_profile(id,created_at) VALUES(?,NOW())", i+1); err != nil {
			t.Fatal(err)
		}
		for j, image := range fixture.Images {
			if _, err := db.Exec(ctx, "INSERT INTO hg_youban_publish_media(profile_id,md5,perceptual_hash,media_type,purpose,sort_index) VALUES(?,?,?,'image','display',?)", i+1, image.Md5, image.PerceptualHash, j); err != nil {
				t.Fatal(err)
			}
		}
	}
	// A matching verification image must not leak into display-image matching.
	if _, err := db.Exec(ctx, "INSERT INTO hg_content_profile(id,created_at) VALUES(4,NOW())"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO hg_youban_publish_media(profile_id,md5,perceptual_hash,media_type,purpose) VALUES(4,?,?,'image','verify')", fixtures[0].Images[0].Md5, fixtures[0].Images[0].PerceptualHash); err != nil {
		t.Fatal(err)
	}
	session := &duplicateScanSession{}
	service := new(sSysPublish)
	for _, id := range []int64{1, 2, 3, 4} {
		if err := service.appendDuplicateScanSignatures(ctx, "sql-fixture", session, []int64{id}); err != nil {
			t.Fatal(err)
		}
		session.ScannedTotal++
	}
	if session.IncompleteTotal != 1 {
		t.Fatalf("verification-only profile was scanned: %+v", session)
	}
	group, err := newDuplicateScanWorkCache(ctx, "sql-fixture").group("phash:3")
	if err != nil || group == nil || !containsInt64(group.MemberIds, 1) || !containsInt64(group.MemberIds, 2) {
		t.Fatalf("SQL scan missed customer duplicates: %+v err=%v", group, err)
	}
	adapter := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
	if err := adapter.Set("cache.adapter", "memory"); err != nil {
		t.Fatal(err)
	}
	cache.SetAdapter(ctx)
	result, err := service.finishDuplicateScan(ctx, "sql-fixture", session)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ScanComplete || result.ScannedTotal != 4 || result.DuplicateTotal != 2 || result.BatchTotal != 2 {
		t.Fatalf("unexpected final search result: %+v", result)
	}
	seen := map[int64]bool{}
	for _, group := range result.Groups {
		for _, candidate := range group.Duplicates {
			seen[candidate.Id] = true
			if !duplicateScanImagesMatch(fixtures[group.Keep.Id-1].Images, fixtures[candidate.Id-1].Images) {
				t.Fatalf("returned indirect match: %+v", group)
			}
		}
	}
	if !seen[1] || !seen[2] || seen[4] {
		t.Fatalf("wrong search candidates: %v", seen)
	}
	for sql, want := range map[string]int{
		"SELECT count(*) FROM hg_content_profile WHERE deleted_at IS NULL":      4,
		"SELECT count(*) FROM hg_youban_publish_media WHERE deleted_at IS NULL": 13,
	} {
		value, err := db.GetValue(ctx, sql)
		if err != nil || value.Int() != want {
			t.Fatalf("scan mutated fixture: %s count=%v want=%d err=%v", sql, value, want, err)
		}
	}
}
