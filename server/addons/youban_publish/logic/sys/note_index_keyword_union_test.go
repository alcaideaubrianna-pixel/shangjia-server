package sys

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestNoteIndexUnionKeywordTerms(t *testing.T) {
	for _, input := range []string{"", "  ", "KK12426", "xxy6400", "123"} {
		if got := noteIndexUnionKeywordTerms(input); len(got) != 0 {
			t.Fatalf("special keyword %q: %v", input, got)
		}
	}
	if got := noteIndexUnionKeywordTerms("Alpha beta Alpha"); !reflect.DeepEqual(got, []string{"Alpha", "beta"}) {
		t.Fatalf("ordinary terms: %v", got)
	}
}

func TestNoteIndexKeywordUnionConditions(t *testing.T) {
	projection, args, fallback, fallbackArgs := noteIndexKeywordUnionConditions([]string{"Alpha", "beta"})
	if strings.Count(projection, "EXISTS (") != 2 || strings.Count(projection, "ILIKE ?") != 2 || !strings.Contains(projection, " AND ") {
		t.Fatalf("projection conditions: %s", projection)
	}
	if !reflect.DeepEqual(args, []interface{}{"%Alpha%", "%beta%"}) {
		t.Fatalf("projection args: %v", args)
	}
	if !strings.Contains(fallback, "NOT EXISTS (") || strings.Count(fallback, " LIKE ?") != 8 || len(fallbackArgs) != 8 {
		t.Fatalf("fallback: %s %v", fallback, fallbackArgs)
	}
}

// Exercises GoFrame SQL generation and parameter binding on an explicitly
// selected disposable local PostgreSQL instance; all tables are temporary.
func TestNoteIndexKeywordUnionPostgres(t *testing.T) {
	link := os.Getenv("YOUBAN_NOTE_INDEX_TEST_PG")
	if link == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	if link != "pgsql:test:@tcp(127.0.0.1:55483)/postgres" {
		t.Fatal("refusing non-test database")
	}
	var groupSuffix [16]byte
	if _, err := rand.Read(groupSuffix[:]); err != nil {
		t.Fatal(err)
	}
	group := "note_keyword_union_test_" + hex.EncodeToString(groupSuffix[:])
	if err := gdb.SetConfigGroup(group, gdb.ConfigGroup{{Link: link}}); err != nil {
		t.Fatal(err)
	}
	db, err := gdb.NewByGroup(group)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	config := db.GetConfig()
	if db.GetGroup() != group || config.Type != "pgsql" || config.Host != "127.0.0.1" || config.Port != "55483" || config.Name != "postgres" || config.User != "test" {
		t.Fatal("refusing unexpected test database configuration")
	}
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		statements := []string{
			"CREATE TEMP TABLE hg_youban_publish_note_index (id bigint, tenant_id bigint, account_id bigint, profile_id bigint, deleted_at timestamp, created_at timestamp, profile_no text, title text, summary text, plain_text text) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_profile_state (tenant_id bigint, account_id bigint, profile_id bigint, deleted_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_note_search (tenant_id bigint NOT NULL, account_id bigint NOT NULL, profile_id bigint NOT NULL, deleted_at timestamp, search_text text, UNIQUE (tenant_id, account_id, profile_id)) ON COMMIT DROP",
			"INSERT INTO hg_youban_publish_note_index VALUES (1,7,8,1,NULL,'2026-01-01','N1','ignored','ignored',''),(2,7,8,2,NULL,'2026-01-02','N2','Alpha','beta',''),(3,7,9,3,NULL,'2026-01-03','N3','Alpha beta','',''),(4,9,8,4,NULL,'2026-01-04','N4','Alpha beta','',''),(5,7,8,5,NULL,'2026-01-05','N5','Alpha beta','',''),(6,7,8,6,NULL,'2026-01-06','N6','Alpha beta','',''),(7,7,8,7,NULL,'2026-01-07','N7','Alpha beta','','')",
			"INSERT INTO hg_youban_publish_profile_state VALUES (7,8,1,NULL),(7,8,2,NULL),(7,9,3,NULL),(9,8,4,NULL),(7,8,5,now()),(7,8,6,NULL),(7,8,7,NULL)",
			"INSERT INTO hg_youban_publish_note_search VALUES (7,8,1,NULL,'ALPHA BETA'),(7,8,6,NULL,'unmatched'),(7,8,7,now(),'unmatched')",
		}
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		base := tx.Model("hg_youban_publish_note_index i").
			InnerJoin("hg_youban_publish_profile_state ps", "ps.tenant_id=i.tenant_id AND ps.account_id=i.account_id AND ps.profile_id=i.profile_id AND ps.deleted_at IS NULL").
			WhereNull("i.deleted_at").Where("i.tenant_id", 7).WhereIn("i.account_id", []int{8, 9}).Where("i.id < ?", 7)
		type row struct {
			NoteIndexId int64 `orm:"note_index_id"`
		}
		var original, rewritten []row
		if err := applyProfileKeywordSearch(base.Clone(), "Alpha beta", noteIndexSearchFields()).
			Fields("i.id AS note_index_id,i.profile_id AS id,i.created_at AS sort_at").
			Order("i.created_at DESC").Order("i.id DESC").Limit(21).Scan(&original); err != nil {
			return err
		}
		if err := noteIndexKeywordUnionQueryDB(ctx, db, base, []string{"Alpha", "beta"}, "i.created_at", "DESC", 21).Scan(&rewritten); err != nil {
			return err
		}
		want := []row{{3}, {2}, {1}}
		if !reflect.DeepEqual(original, want) || !reflect.DeepEqual(rewritten, original) {
			t.Fatalf("original=%v union=%v want=%v", original, rewritten, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
