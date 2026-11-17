package sys

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// Uses only transaction-local fixtures on an explicitly selected loopback PG.
// Owner rows deliberately lack source_type, as media/history rows may do.
func TestCollectRecoverySourceAccountLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	previousDB := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres",
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_source(
				id bigint PRIMARY KEY, tenant_id bigint, account_id bigint,
				source_type text, collect_enabled smallint, status smallint,
				deleted_at timestamp
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE collect_recovery_source_account_owner(
				id bigint PRIMARY KEY, tenant_id bigint, account_id bigint,
				source_id bigint, status text DEFAULT 'pending'
			) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_source VALUES
				(351,78,615,'bot',1,1,NULL),
				(352,78,615,'account',1,1,NULL),
				(353,78,615,'bot',0,1,NULL),
				(354,78,615,'bot',1,2,NULL),
				(355,78,615,'bot',1,1,NOW()),
				(356,79,615,'bot',1,1,NULL),
				(357,78,0,'bot',1,1,NULL)`,
			`INSERT INTO collect_recovery_source_account_owner
				(id,tenant_id,account_id,source_id) VALUES
				(1,78,615,351),(2,78,0,351),(3,78,616,351),
				(4,78,0,352),(5,78,615,352),(6,78,0,353),
				(7,78,0,354),(8,78,0,355),(9,78,0,356),
				(10,78,0,999),(11,78,616,352),(12,78,-1,351),
				(13,78,0,357)`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		cases := []struct {
			name    string
			id      int64
			enabled bool
		}{
			{"exact_bot_owner", 1, true},
			{"bot_raw_zero", 2, true},
			{"different_positive_bot_owner", 3, false},
			{"non_bot_raw_zero", 4, false},
			{"exact_non_bot_owner", 5, true},
			{"collection_disabled", 6, false},
			{"source_disabled", 7, false},
			{"source_deleted", 8, false},
			{"different_tenant", 9, false},
			{"missing_source", 10, false},
			{"different_positive_non_bot_owner", 11, false},
			{"negative_account", 12, false},
			// Existing exact 0=0 behavior remains unchanged.
			{"exact_zero_owner", 13, true},
		}
		for _, alias := range []string{"e", "m", "h"} {
			for _, tc := range cases {
				t.Run(alias+"/"+tc.name, func(t *testing.T) {
					value, err := tx.GetValue("SELECT "+enabledCollectSourceExistsSQL(alias)+
						" FROM collect_recovery_source_account_owner "+alias+" WHERE "+alias+".id=?", tc.id)
					if err != nil {
						t.Fatal(err)
					}
					if value.Bool() != tc.enabled {
						t.Fatalf("enabled=%v, want %v", value.Bool(), tc.enabled)
					}
				})
			}
		}
		// Exercise the disabled-source cleanup predicate itself: valid raw0
		// Bot events must remain pending, while invalid scopes are ignored.
		if _, err := tx.Exec("UPDATE collect_recovery_source_account_owner e SET status='ignored' WHERE NOT " +
			enabledCollectSourceExistsSQL("e")); err != nil {
			return err
		}
		for _, tc := range cases {
			value, err := tx.GetValue("SELECT status FROM collect_recovery_source_account_owner WHERE id=?", tc.id)
			if err != nil {
				return err
			}
			want := "ignored"
			if tc.enabled {
				want = "pending"
			}
			if value.String() != want {
				t.Errorf("%s cleanup status=%q, want %q", tc.name, value.String(), want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
