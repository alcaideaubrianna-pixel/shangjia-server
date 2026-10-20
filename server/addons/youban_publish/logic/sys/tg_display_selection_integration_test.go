package sys

import (
	"context"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

// Only a dedicated, disposable local cluster is accepted. No Telegram calls,
// Redis, production configuration, or queue consumers are used.
func TestDisplaySelectionPostgres(t *testing.T) {
	link := os.Getenv("YOUBAN_RANDOM_MEDIA_TEST_PG")
	if link == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	if link != "pgsql:test:@tcp(127.0.0.1:55483)/postgres" {
		t.Fatal("refusing non-test database")
	}
	before := gdb.GetConfig("default")
	beforeCfg := g.Cfg().GetAdapter()
	t.Cleanup(func() { gdb.SetConfigGroup("default", before); g.Cfg().SetAdapter(beforeCfg) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Link: link}})
	adapter, err := gcfg.NewAdapterContent(`{"cache":{"adapter":"memory"},"database":{"default":{"link":"pgsql:test:@tcp(127.0.0.1:55483)/postgres"}}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	ctx := context.Background()
	cache.SetAdapter(ctx)
	db := g.DB()
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS hg_youban_publish_tg_attempt,hg_youban_publish_profile_state,hg_youban_publish_tg_job")
	})
	for _, ddl := range []string{
		"CREATE TABLE hg_youban_publish_tg_job(id bigint PRIMARY KEY,send_phase text NOT NULL DEFAULT '',updated_at timestamp)",
		"CREATE TABLE hg_youban_publish_profile_state(profile_id bigint PRIMARY KEY,tenant_id bigint)",
		"CREATE TABLE hg_youban_publish_tg_attempt(id bigint,job_id bigint,phase text)",
		"INSERT INTO hg_youban_publish_tg_job(id) VALUES(1),(2),(3),(4)",
		"INSERT INTO hg_youban_publish_profile_state VALUES(7,9)",
	} {
		if _, err = db.Exec(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	migration, err := os.ReadFile("../../resource/sql/random_display_media.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = db.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("migration run %d: %v", i, err)
		}
	}
	if err = cache.Instance().Set(ctx, tenantVipCacheKey(9), &sysin.TenantVipStatusModel{TenantId: 9, IsVip: true, Features: []string{sysin.TenantVipFeatureRandomMedia}}, time.Minute); err != nil {
		t.Fatal(err)
	}
	media := make([]*telegramMediaItem, 20)
	for i := range media {
		media[i] = &telegramMediaItem{Id: int64(i + 1), MediaType: "image", Purpose: "display"}
	}
	s := new(sSysPublish)
	job := telegramJobRecord{Id: 1, TenantId: 9, ProfileId: 7}
	var wg sync.WaitGroup
	results := make([][]*telegramMediaItem, 12)
	errors := make([]error, len(results))
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errors[i] = s.selectTelegramDisplayMediaForTenant(ctx, job, media)
		}(i)
	}
	wg.Wait()
	for i := range results {
		if errors[i] != nil || len(results[i]) != 10 || !reflect.DeepEqual(results[i], results[0]) {
			t.Fatalf("concurrent selection %d: %v", i, errors[i])
		}
	}
	on := true
	if err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		return saveProfileRandomMediaTx(ctx, tx, 7, &sysin.ProfileRandomMediaPolicy{Enabled: &on, Count: 3})
	}); err != nil {
		t.Fatal(err)
	}
	// Older clients omitting policy must not overwrite the saved configuration.
	if err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error { return saveProfileRandomMediaTx(ctx, tx, 7, nil) }); err != nil {
		t.Fatal(err)
	}
	again, err := s.selectTelegramDisplayMediaForTenant(ctx, job, media)
	if err != nil || !reflect.DeepEqual(again, results[0]) {
		t.Fatalf("profile edit changed an existing job: %v", err)
	}
	job.Id = 2
	got, err := s.selectTelegramDisplayMediaForTenant(ctx, job, media)
	if err != nil || len(got) != 3 {
		t.Fatalf("new job should use edited policy: %v, %d", err, len(got))
	}
	// Existing attempts use the legacy ordering/selection rather than applying
	// a new three-image policy to an already-started album.
	if _, err = db.Exec(ctx, "INSERT INTO hg_youban_publish_tg_attempt VALUES(1,3,'display')"); err != nil {
		t.Fatal(err)
	}
	job.Id = 3
	got, err = s.selectTelegramDisplayMediaForTenant(ctx, job, media)
	want := selectTelegramDisplayMedia(job, media, 10, true)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy attempt selection changed: %v", err)
	}
	if _, err = s.selectTelegramDisplayMediaForTenant(ctx, telegramJobRecord{Id: 999}, media); err == nil {
		t.Fatal("missing job must fail before sending")
	}
	job.Id = 4
	if _, err = db.Exec(ctx, "ALTER TABLE hg_youban_publish_tg_job ADD CONSTRAINT test_snapshot_failure CHECK(id<>4 OR display_media_selection='')"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.selectTelegramDisplayMediaForTenant(ctx, job, media); err == nil {
		t.Fatal("snapshot persistence failure must prevent sending")
	}
	value, err := db.Model(publishTgJobTable).Where("id", 4).Value("display_media_selection")
	if err != nil || value.String() != "" {
		t.Fatal("failed snapshot update must roll back")
	}
}
