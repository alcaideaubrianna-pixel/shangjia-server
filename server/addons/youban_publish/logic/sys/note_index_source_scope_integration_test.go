package sys

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

// Uses an isolated schema on a disposable local cluster, never production data.
func TestNoteIndexSourceScopePostgres(t *testing.T) {
	link := os.Getenv("YOUBAN_NOTE_INDEX_TEST_PG")
	if link == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	if link != "pgsql:test:@tcp(127.0.0.1:55483)/postgres" {
		t.Fatal("refusing non-test database")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Link: link}})
	ctx := context.Background()
	db := g.DB()
	err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE SCHEMA note_source_scope_test",
			"SET LOCAL search_path=note_source_scope_test,public",
			"CREATE TABLE hg_content_profile(id bigint PRIMARY KEY,source_type text,profile_no text,deleted_at timestamp)",
			"CREATE TABLE hg_youban_publish_note_index(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,created_at timestamp,deleted_at timestamp)",
			"CREATE TABLE hg_youban_publish_profile_state(profile_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at timestamp)",
			"INSERT INTO hg_content_profile VALUES(1,'youban_collect','C1',NULL),(2,'manual','M2',NULL),(3,NULL,'M3',NULL),(4,'youban_collect','C4',NULL),(5,'youban_collect','C5',NULL)",
			"INSERT INTO hg_youban_publish_profile_state VALUES(1,7,8,NULL),(2,7,8,NULL),(3,7,8,NULL),(4,9,10,NULL),(5,7,8,now())",
			"INSERT INTO hg_youban_publish_note_index VALUES(1,7,8,1,'2026-10-20',NULL),(2,7,8,2,'2026-10-19',NULL),(3,7,8,3,'2026-10-18',NULL),(4,9,10,4,'2026-10-17',NULL),(5,7,8,5,'2026-10-16',NULL)",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		migration, err := os.ReadFile(filepath.Join("..", "..", "resource", "sql", "note_index_source_scope.pgsql.sql"))
		if err != nil {
			return err
		}
		// Keep the migration inside this rollback-only test transaction.
		sql := strings.ReplaceAll(string(migration), "BEGIN;", "")
		sql = strings.ReplaceAll(sql, "COMMIT;", "")
		for i := 0; i < 2; i++ {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("UPDATE hg_youban_publish_note_index i SET is_collected=COALESCE(p.source_type='youban_collect',false) FROM hg_content_profile p WHERE p.id=i.profile_id"); err != nil {
			return err
		}
		assertIDs := func(scope string, want string) {
			t.Helper()
			mod := tx.Model(publishNoteIndexTable+" i").
				InnerJoin("hg_content_profile p", "p.id=i.profile_id AND p.deleted_at IS NULL").
				InnerJoin("hg_youban_publish_profile_state ps", "ps.profile_id=i.profile_id AND ps.tenant_id=i.tenant_id AND ps.account_id=i.account_id AND ps.deleted_at IS NULL").
				WhereNull("i.deleted_at")
			mod = applyNoteIndexScope(mod, 7, nil, []int64{8}, &sysin.ProfileListInp{})
			mod = applyNoteIndexFilters(mod, &sysin.ProfileListInp{SourceScope: scope})
			values, err := mod.OrderAsc("i.id").Array("i.id")
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, value := range values {
				ids = append(ids, value.String())
			}
			if got := strings.Join(ids, ","); got != want {
				t.Fatalf("scope %s: got %s, want %s", scope, got, want)
			}
		}
		assertIDs("collected", "1")
		assertIDs("manual", "2,3")
		assertIDs("", "1,2,3")
		// Old writers omit the new column; the trigger fills it automatically.
		if _, err := tx.Exec("INSERT INTO hg_youban_publish_note_index(id,tenant_id,account_id,profile_id) VALUES(6,7,8,1)"); err != nil {
			return err
		}
		assertIDs("collected", "1,6")
		// Media/profile deletion still uses the authoritative joined tables.
		if _, err := tx.Exec("UPDATE hg_content_profile SET deleted_at=now() WHERE id=1"); err != nil {
			return err
		}
		assertIDs("collected", "")
		if _, err := tx.Exec("UPDATE hg_content_profile SET deleted_at=NULL WHERE id=1"); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE hg_content_profile SET source_type='manual' WHERE id=1"); err != nil {
			return err
		}
		assertIDs("collected", "")
		if _, err := tx.Exec("UPDATE hg_content_profile SET source_type='youban_collect' WHERE id=2"); err != nil {
			return err
		}
		assertIDs("collected", "2")
		// Replacing the profile reference recomputes the source marker.
		if _, err := tx.Exec("UPDATE hg_youban_publish_note_index SET profile_id=2 WHERE id=6"); err != nil {
			return err
		}
		assertIDs("collected", "2,6")
		if _, err := tx.Exec("UPDATE hg_youban_publish_note_index SET deleted_at=now() WHERE id=6"); err != nil {
			return err
		}
		assertIDs("collected", "2")
		if _, err := tx.Exec("UPDATE hg_youban_publish_note_index SET is_collected=false WHERE id=2"); err != nil {
			return err
		}
		assertIDs("collected", "2")
		return tx.Rollback()
	})
	if err != nil && !strings.Contains(err.Error(), "transaction has already been committed or rolled back") {
		t.Fatal(err)
	}
}
