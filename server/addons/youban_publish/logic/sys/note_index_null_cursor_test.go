package sys

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"os"
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestNoteNullCursorCompatibility(t *testing.T) {
	legacy := base64.RawURLEncoding.EncodeToString([]byte(`{"indexId":5,"sortBy":"createdAtAsc","sortAt":"2026-01-01T00:00:00Z"}`))
	cursor, err := decodeAdminNoteCursor(legacy)
	if err != nil || cursor == nil || cursor.SortNull || cursor.IndexId != 5 {
		t.Fatalf("legacy cursor: %+v, %v", cursor, err)
	}
	for _, sortBy := range []string{"createdAtAsc", "createdAtDesc", "updatedAtAsc", "updatedAtDesc"} {
		raw := encodeAdminNotePageCursor(5, nil, sortBy)
		cursor, err := decodeAdminNoteCursor(raw)
		if err != nil || cursor == nil || !cursor.SortNull || cursor.IndexId != 5 {
			t.Fatalf("%s null cursor: %+v, %v", sortBy, cursor, err)
		}
	}
	for _, payload := range []string{
		`{"indexId":5,"sortBy":"createdAtAsc","sortAt":""}`,
		`{"indexId":5,"sortBy":"createdAtAsc","sortAt":"2026-01-01T00:00:00Z","sortNull":true}`,
	} {
		if _, err := decodeAdminNoteCursor(base64.RawURLEncoding.EncodeToString([]byte(payload))); err == nil {
			t.Fatalf("accepted invalid cursor: %s", payload)
		}
	}
	if _, _, err := adminNoteCursorCondition(&adminNoteCursor{IndexId: 5, SortNull: true}, "publishedAtAsc"); err == nil {
		t.Fatal("published COALESCE cannot have a NULL cursor")
	}
}

// Uses only the explicitly configured disposable local PostgreSQL instance.
func TestNoteNullCursorPostgresPages(t *testing.T) {
	link := os.Getenv("YOUBAN_NOTE_INDEX_TEST_PG")
	if link == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	if link != "pgsql:test:@tcp(127.0.0.1:55483)/postgres" {
		t.Fatal("refusing non-test database")
	}
	var suffix [16]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	group := "note_null_cursor_test_" + hex.EncodeToString(suffix[:])
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
		connection, err := tx.GetOne("SELECT host(inet_server_addr()) AS server_addr, inet_server_port() AS server_port, current_database() AS db_name, current_user AS db_user")
		if err != nil {
			return err
		}
		if connection["server_addr"].String() != "127.0.0.1" || connection["server_port"].Int() != 55483 || connection["db_name"].String() != "postgres" || connection["db_user"].String() != "test" {
			t.Fatal("refusing unexpected PostgreSQL connection")
		}
		if _, err := tx.Exec("CREATE TEMP TABLE note_cursor_test (id bigint PRIMARY KEY, created_at timestamp, updated_at timestamp, published_at timestamp) ON COMMIT DROP"); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO note_cursor_test VALUES (1,NULL,NULL,NULL),(2,NULL,NULL,NULL),(3,'2026-01-01','2026-01-01',NULL),(4,'2026-01-01','2026-01-01','1970-01-01'),(5,'2026-02-01','2026-02-01','2026-02-01')"); err != nil {
			return err
		}
		for _, tc := range []struct {
			sortBy string
			want   []int64
		}{
			{"createdAtDesc", []int64{2, 1, 5, 4, 3}},
			{"createdAtAsc", []int64{3, 4, 5, 1, 2}},
			{"updatedAtDesc", []int64{2, 1, 5, 4, 3}},
			{"updatedAtAsc", []int64{3, 4, 5, 1, 2}},
			{"publishedAtDesc", []int64{5, 4, 3, 2, 1}},
			{"publishedAtAsc", []int64{1, 2, 3, 4, 5}},
		} {
			var ids []int64
			cursor := ""
			for page := 0; page < 10; page++ {
				mod, err := applyNoteIndexCursor(tx.Model("note_cursor_test i"), cursor, tc.sortBy)
				if err != nil {
					return err
				}
				var rows []struct {
					Id     int64       `orm:"id"`
					SortAt *gtime.Time `orm:"sort_at"`
				}
				expression := adminNoteSortExpression(tc.sortBy)
				if err := mod.Fields("i.id," + expression + " AS sort_at").Order(expression + " " + adminNoteSortDirection(tc.sortBy)).Order("i.id " + adminNoteSortDirection(tc.sortBy)).Limit(2).Scan(&rows); err != nil {
					return err
				}
				if len(rows) == 0 {
					break
				}
				for _, row := range rows {
					ids = append(ids, row.Id)
				}
				last := rows[len(rows)-1]
				cursor = encodeAdminNotePageCursor(last.Id, last.SortAt, tc.sortBy)
				if cursor == "" {
					t.Fatalf("%s: empty cursor at %d", tc.sortBy, last.Id)
				}
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("%s: got %v, want %v", tc.sortBy, ids, tc.want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
