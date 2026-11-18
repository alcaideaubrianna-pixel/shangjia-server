package sys

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

// These admission cases must return before any identity/asset/database lookup.
// A canceled context catches accidental I/O without configuring a database.
func TestMergeCollectDuplicateVerificationAdmission(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name    string
		review  int
		content *collectContentResult
	}{
		{name: "nil content"},
		{name: "empty media", content: &collectContentResult{}},
		{name: "display video is not verification", content: &collectContentResult{Media: []collectMediaItem{{Type: "video", Purpose: "display"}}}},
		{name: "verification image is not video", content: &collectContentResult{Media: []collectMediaItem{{Type: "photo", Purpose: "verify"}}}},
		{name: "review branch cannot automatically merge", review: 1, content: &collectContentResult{Media: []collectMediaItem{{Type: "video", Purpose: "verify"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := gdb.Record{"review_enabled": g.NewVar(tc.review)}
			if err := (&sSysPublish{}).mergeCollectDuplicateVerification(ctx, nil, rule, tc.content); err != nil {
				t.Fatalf("ineligible merge should stop before I/O: %v", err)
			}
		})
	}
}

// This fixture deliberately uses a disposable schema instead of the production
// database and never reads a DSN, Redis address, or application configuration.
// The implementation under test is expected to use the contract table through
// the current search_path.
func TestRecordCollectDuplicateIdentityLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}

	previous := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previous) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres"}})
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: memory\nyoubanPublish:\n  profileOriginEnabled: true\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	schema := fmt.Sprintf("yb_identity_test_%d", time.Now().UnixNano())
	if _, err = g.DB().Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = g.DB().Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`) })
	if err = createCollectIdentityFixture(ctx, schema); err != nil {
		t.Fatal(err)
	}

	service := &sSysPublish{}
	event := gdb.Record{
		"id": g.NewVar(int64(20303280)), "tenant_id": g.NewVar(int64(78)), "account_id": g.NewVar(int64(615)),
		"source_id": g.NewVar(int64(351)), "source_chat_id": g.NewVar("-1003763589169"),
	}
	rule := gdb.Record{"id": g.NewVar(int64(338)), collectRuleSourceIDField: g.NewVar(int64(351))}
	hit := &profileFingerprintDuplicateError{ProfileID: 844309, Layer: "text_hash"}

	// Ten workers record the same logical event. The identity primary key must
	// make this idempotent, while the owner-state check must still be enforced.
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- withCollectIdentitySearchPath(ctx, schema, func(txCtx context.Context) error {
				return service.recordCollectDuplicateIdentity(txCtx, event, rule, hit)
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("same identity must be idempotent: %v", err)
		}
	}

	var count int
	if err = withCollectIdentitySearchPath(ctx, schema, func(txCtx context.Context) error {
		var err error
		count, err = g.DB().Model("hg_youban_publish_collect_identity").Ctx(txCtx).Where("tenant_id", 78).Where("account_id", 615).Where("source_id", 351).Where("rule_id", 338).Where("event_id", 20303280).Count()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("same identity rows=%d, want 1", count)
	}
	if err = withCollectIdentitySearchPath(ctx, schema, func(txCtx context.Context) error {
		row, err := g.DB().Model("hg_youban_publish_collect_identity").Ctx(txCtx).
			Where("event_id", 20303280).Where("rule_id", 338).One()
		if err != nil {
			return err
		}
		if row["profile_id"].Int64() != 844309 || row["canonical_event_id"].Int64() != 20303151 {
			return fmt.Errorf("M19327 identity lost: %v", row)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("four independent worker binaries", func(t *testing.T) {
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		var workers sync.WaitGroup
		failures := make(chan string, 4)
		for i := 0; i < 4; i++ {
			workers.Add(1)
			go func() {
				defer workers.Done()
				cmd := exec.CommandContext(ctx, binary, "-test.run=^TestCollectIdentityWorkerProcess$", "-test.timeout=20s")
				cmd.Env = append(os.Environ(), "YOUBAN_IDENTITY_CHILD_SCHEMA="+schema)
				out, err := cmd.CombinedOutput()
				if err != nil {
					failures <- fmt.Sprintf("%v %s", err, out)
				}
			}()
		}
		workers.Wait()
		close(failures)
		for failure := range failures {
			t.Error(failure)
		}
	})
	// The same event cannot be remapped to another profile.
	conflict := *hit
	conflict.ProfileID = 844310
	if err = withCollectIdentitySearchPath(ctx, schema, func(txCtx context.Context) error {
		return service.recordCollectDuplicateIdentity(txCtx, event, rule, &conflict)
	}); err == nil {
		t.Fatal("different profile for the same identity must fail")
	}

	// A different rule is a different identity namespace.
	otherRule := gdb.Record{"id": g.NewVar(int64(339)), collectRuleSourceIDField: g.NewVar(int64(351))}
	if err = withCollectIdentitySearchPath(ctx, schema, func(txCtx context.Context) error {
		return service.recordCollectDuplicateIdentity(txCtx, event, otherRule, hit)
	}); err != nil {
		t.Fatalf("different rule must remain isolated: %v", err)
	}
	t.Run("cross source retains profile only", func(t *testing.T) {
		// Keep this case in the same fixture shape as the production M19327 pair:
		// ignored display 20303280, verify 20303409, canonical display 20303151.
		// The implementation must persist profile_id but canonical_event_id=0 when
		// the hit cannot be proven to belong to the event's source/rule branch.
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		service := &sSysPublish{}
		event := gdb.Record{"id": g.NewVar(int64(20303409)), "tenant_id": g.NewVar(int64(78)), "account_id": g.NewVar(int64(615)), "source_id": g.NewVar(int64(999)), "source_chat_id": g.NewVar("-1003763589169")}
		rule := gdb.Record{"id": g.NewVar(int64(338)), collectRuleSourceIDField: g.NewVar(int64(999))}
		hit := &profileFingerprintDuplicateError{ProfileID: 844309, Layer: "text_hash"}
		if err := withCollectIdentitySearchPath(ctx, schema, func(txCtx context.Context) error {
			return service.recordCollectDuplicateIdentity(txCtx, event, rule, hit)
		}); err != nil {
			t.Fatalf("cross-source profile mapping should be retained without canonical: %v", err)
		}
		var canonical int64
		if err := withCollectIdentitySearchPath(ctx, schema, func(txCtx context.Context) error {
			value, err := g.DB().Model("hg_youban_publish_collect_identity").Ctx(txCtx).Where("event_id", 20303409).Value("canonical_event_id")
			canonical = value.Int64()
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if canonical != 0 {
			t.Fatalf("cross-source identity canonical_event_id=%d, want 0", canonical)
		}
	})
}

func withCollectIdentitySearchPath(ctx context.Context, schema string, fn func(context.Context) error) error {
	return g.DB().Transaction(ctx, func(txCtx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`SET LOCAL search_path TO ` + schema); err != nil {
			return err
		}
		return fn(txCtx)
	})
}

func createCollectIdentityFixture(ctx context.Context, schema string) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		_, err := tx.Exec(`SET LOCAL search_path TO ` + schema)
		if err != nil {
			return err
		}
		for _, statement := range []string{
			`CREATE TABLE hg_youban_publish_collect_identity(tenant_id bigint NOT NULL,account_id bigint NOT NULL,source_id bigint NOT NULL,rule_id bigint NOT NULL,event_id bigint NOT NULL,profile_id bigint NOT NULL,canonical_event_id bigint NOT NULL DEFAULT 0,source_chat_id text NOT NULL DEFAULT '',match_layer text NOT NULL DEFAULT '',created_at timestamp,updated_at timestamp,PRIMARY KEY(tenant_id,account_id,source_id,rule_id,event_id))`,
			`CREATE TABLE hg_youban_publish_collect_event(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,source_chat_id text,material_group_status text,updated_at timestamp)`,
			`CREATE TABLE hg_youban_publish_collect_dispatch(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,event_id bigint,profile_id bigint,source_chat_id_snapshot text,status text)`,
			`CREATE TABLE hg_youban_publish_profile_state(profile_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at timestamp)`,
			`INSERT INTO hg_youban_publish_profile_state VALUES(844309,78,615,NULL),(844310,78,615,NULL)`,
			`INSERT INTO hg_youban_publish_collect_event(id,tenant_id,account_id,source_id,source_chat_id) VALUES(20303151,78,615,351,'-1003763589169'),(20303280,78,615,351,'-1003763589169'),(20303409,78,615,999,'-1003763589169')`,
			`INSERT INTO hg_youban_publish_collect_dispatch VALUES(1,78,615,351,338,20303151,844309,'-1003763589169','sent')`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		return nil
	})
}

func TestCollectIdentityPreparedMediaConcurrentCommit(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid local port")
	}
	previous := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", previous)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	schema := fmt.Sprintf("yb_identity_media_%d", time.Now().UnixNano())
	if _, err := g.DB().Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer g.DB().Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	if err := createCollectIdentityFixture(ctx, schema); err != nil {
		t.Fatal(err)
	}
	err := withCollectIdentitySearchPath(ctx, schema, func(ctx context.Context) error {
		for _, q := range []string{
			`CREATE TABLE hg_youban_publish_media(id bigserial PRIMARY KEY,profile_id bigint,tenant_id bigint,merchant_id bigint,account_id bigint,media_type text,purpose text,name text,tg_file_id text,tg_cache_asset_hash text,tg_cache_status text,size bigint,status int,sort_index int,attachment_id bigint DEFAULT 0,original_attachment_id bigint DEFAULT 0,original_storage_path text,storage_path text,file_url text,poster_url text,poster_storage_path text,md5 text,perceptual_hash text,deleted_at timestamp,created_by bigint,updated_by bigint,created_at timestamp,updated_at timestamp)`,
			`CREATE TABLE hg_content_media(id bigserial PRIMARY KEY,profile_id bigint,source_asset_id bigint,media_type text,purpose text,sort_index int,original_storage_path text,display_storage_path text,preview_storage_path text,binary_md5 text,perceptual_hash text,process_status text,encrypt_status text,status int,deleted_at timestamp,created_at timestamp,updated_at timestamp)`,
			`CREATE TABLE hg_content_profile(id bigint PRIMARY KEY,image_count int,video_count int,has_verification_video int,cover_media_id bigint,updated_at timestamp)`,
			`INSERT INTO hg_content_profile(id) VALUES(844309)`,
			`INSERT INTO hg_youban_publish_media(profile_id,media_type,purpose,sort_index,storage_path) VALUES(844309,'image','display',1,'original-display.jpg')`,
		} {
			if _, err := g.DB().Exec(ctx, q); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	event := gdb.Record{"id": g.NewVar(20303280), "tenant_id": g.NewVar(78), "account_id": g.NewVar(615)}
	prepared := []collectPreparedMedia{{MediaType: "image", Purpose: "display", StoragePath: "must-not-replace.jpg"}, {MediaType: "video", Purpose: "verify", StoragePath: "verification.mp4", SortIndex: 1}}
	errs := make(chan error, 12)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- withCollectIdentitySearchPath(ctx, schema, func(ctx context.Context) error {
				return (&sSysPublish{}).mergeCollectIdentityPreparedVerification(ctx, event, 844309, prepared)
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := withCollectIdentitySearchPath(ctx, schema, func(ctx context.Context) error {
		row, err := g.DB().GetOne(ctx, `SELECT (SELECT count(*) FROM hg_youban_publish_media WHERE purpose='verify') n,(SELECT storage_path FROM hg_youban_publish_media WHERE purpose='display') display,has_verification_video FROM hg_content_profile WHERE id=844309`)
		if err != nil {
			return err
		}
		if row["n"].Int() != 1 || row["display"].String() != "original-display.jpg" || row["has_verification_video"].Int() != 1 {
			t.Errorf("invalid merged projection: %v", row)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCollectIdentityWorkerProcess(t *testing.T) {
	schema := os.Getenv("YOUBAN_IDENTITY_CHILD_SCHEMA")
	if schema == "" {
		t.Skip("child worker only")
	}
	if !strings.HasPrefix(schema, "yb_identity_test_") {
		t.Fatal("invalid test schema")
	}
	if _, err := strconv.ParseInt(strings.TrimPrefix(schema, "yb_identity_test_"), 10, 64); err != nil {
		t.Fatal(err)
	}
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	n, err := strconv.Atoi(port)
	if err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid local port")
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	event := gdb.Record{"id": g.NewVar(20303280), "tenant_id": g.NewVar(78), "account_id": g.NewVar(615), "source_id": g.NewVar(351), "source_chat_id": g.NewVar("-1003763589169")}
	rule := gdb.Record{"id": g.NewVar(338), collectRuleSourceIDField: g.NewVar(351)}
	if err := withCollectIdentitySearchPath(ctx, schema, func(ctx context.Context) error {
		return (&sSysPublish{}).recordCollectDuplicateIdentity(ctx, event, rule, &profileFingerprintDuplicateError{ProfileID: 844309, Layer: "text_hash"})
	}); err != nil {
		t.Fatal(err)
	}
}
