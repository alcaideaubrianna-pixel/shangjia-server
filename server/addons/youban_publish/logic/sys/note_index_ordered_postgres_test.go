package sys

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// Both queries execute on one isolated connection against temporary tables only.
func TestNoteIndexOrderedSubqueryPostgres(t *testing.T) {
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
	group := "note_ordered_test_" + hex.EncodeToString(suffix[:])
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
			"CREATE TEMP TABLE hg_youban_publish_note_index (id bigint, tenant_id bigint, account_id bigint, profile_id bigint, deleted_at timestamp, created_at timestamp, is_collected boolean) ON COMMIT DROP",
			"CREATE TEMP TABLE hg_youban_publish_profile_state (tenant_id bigint, account_id bigint, profile_id bigint, deleted_at timestamp) ON COMMIT DROP",
			"INSERT INTO hg_youban_publish_note_index VALUES (10,7,8,10,NULL,'2026-01-03',true),(9,7,9,9,NULL,'2026-01-03',true),(8,7,10,8,NULL,'2026-01-04',true),(7,7,8,7,now(),'2026-01-05',true),(6,7,9,6,NULL,'2026-01-06',true),(5,7,8,5,NULL,'2026-01-07',false),(4,8,8,4,NULL,'2026-01-08',true),(3,7,8,3,NULL,'2026-01-02',true),(2,7,9,2,NULL,NULL,true),(1,7,8,1,NULL,NULL,true)",
			"INSERT INTO hg_youban_publish_profile_state VALUES (7,8,10,NULL),(7,9,9,NULL),(7,10,8,NULL),(7,8,7,NULL),(7,9,6,now()),(7,8,5,NULL),(8,8,4,NULL),(7,8,3,NULL),(7,9,2,NULL),(7,8,1,NULL)",
		}
		for _, statement := range statements {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		const sortBy = "createdAtDesc"
		at := gtime.NewFromTime(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
		cases := []struct {
			name, cursor string
			accounts     []int64
			limit        int
			want         []int64
		}{
			{"multiple accounts limited", "", []int64{8, 9}, 2, []int64{2, 1}},
			{"same timestamp cursor", encodeAdminNotePageCursor(10, at, sortBy), []int64{8, 9}, 10, []int64{9, 3}},
			{"null cursor", encodeAdminNotePageCursor(2, nil, sortBy), []int64{8, 9}, 10, []int64{1, 10, 9, 3}},
			{"single account", "", []int64{9}, 10, []int64{2, 9}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				filters := &sysin.ProfileListInp{SourceScope: "collected"}
				old := tx.Model("hg_youban_publish_note_index i").InnerJoin("hg_youban_publish_profile_state ps", "ps.tenant_id=i.tenant_id AND ps.account_id=i.account_id AND ps.profile_id=i.profile_id AND ps.deleted_at IS NULL").WhereNull("i.deleted_at")
				old = applyNoteIndexFilters(applyNoteIndexScope(old, 7, nil, tc.accounts, filters), filters)
				old, err = applyNoteIndexCursor(old, tc.cursor, sortBy)
				if err != nil {
					t.Fatal(err)
				}
				inner := tx.Model("hg_youban_publish_note_index i").WhereNull("i.deleted_at").Where("i."+noteIndexCollectedColumn, true)
				inner = applyNoteIndexScope(inner, 7, nil, nil, nil)
				inner, err = applyNoteIndexCursor(inner, tc.cursor, sortBy)
				if err != nil {
					t.Fatal(err)
				}
				inner = inner.Order("i.created_at DESC").Order("i.id DESC").Offset(0)
				optimized := db.Model("? AS i", inner).Ctx(ctx).WhereNull("i.deleted_at").Where("EXISTS (SELECT 1 FROM " + publishProfileStateTable + " ps WHERE ps.tenant_id=i.tenant_id AND ps.account_id=i.account_id AND ps.profile_id=i.profile_id AND ps.deleted_at IS NULL)")
				optimized = applyNoteIndexFilters(applyNoteIndexScope(optimized, 7, nil, tc.accounts, filters), filters)
				optimized, err = applyNoteIndexCursor(optimized, tc.cursor, sortBy)
				if err != nil {
					t.Fatal(err)
				}
				type row struct {
					NoteIndexId int64 `orm:"note_index_id"`
				}
				query := func(mod *gdb.Model) []int64 {
					var rows []row
					if err := mod.Fields("i.id AS note_index_id,i.profile_id AS id,i.created_at AS sort_at").Order("i.created_at DESC").Order("i.id DESC").Limit(tc.limit).Scan(&rows); err != nil {
						t.Fatal(err)
					}
					ids := make([]int64, 0, len(rows))
					for _, row := range rows {
						ids = append(ids, row.NoteIndexId)
					}
					return ids
				}
				original, got := query(old), query(optimized)
				if !reflect.DeepEqual(original, tc.want) || !reflect.DeepEqual(got, original) {
					t.Fatalf("original=%v optimized=%v want=%v", original, got, tc.want)
				}
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
