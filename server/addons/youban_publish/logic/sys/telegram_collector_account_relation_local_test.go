package sys

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountCollectChannelRelationLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "resource", "sql", "install.pgsql.sql"))
	if err != nil {
		t.Fatal(err)
	}
	ddl := accountRelationLocalDDL(t, string(raw))
	for _, tc := range []struct {
		name, status, approval                                                 string
		sourceMissing, sourceDisabled, sourceStatusDisabled, deleted, existing bool
		wantAllowed                                                            bool
		interleavingWinner                                                     bool
	}{
		{name: "missing_enabled_creates_and_repeats", wantAllowed: true},
		{name: "interleaving_simulation_disabled_winner", interleavingWinner: true},
		{name: "existing_disabled", existing: true, status: "disabled", approval: "approved"},
		{name: "existing_rejected", existing: true, status: "enabled", approval: "rejected"},
		{name: "existing_pending", existing: true, status: "pending", approval: "pending"},
		{name: "soft_deleted_tombstone", existing: true, status: "enabled", approval: "approved", deleted: true},
		{name: "missing_source", sourceMissing: true},
		{name: "source_collect_disabled", sourceDisabled: true},
		{name: "source_status_disabled", sourceStatusDisabled: true},
		{name: "disabled_source_existing_approved", sourceDisabled: true, existing: true, status: "enabled", approval: "approved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				for _, sql := range ddl {
					if _, err := tx.Exec(sql); err != nil {
						return err
					}
				}
				if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
					return err
				}
				const source = "hg_youban_publish_collect_source"
				const channel = "hg_youban_publish_collect_channel"
				const relation = "hg_youban_publish_collect_account_channel"
				if !tc.sourceMissing {
					enabled, status := 1, 1
					if tc.sourceDisabled {
						enabled = 0
					}
					if tc.sourceStatusDisabled {
						status = 0
					}
					if _, err := tx.Exec("INSERT INTO "+source+" (id,tenant_id,account_id,source_type,source_chat_id,collect_enabled,status) VALUES (101,23,469,'account','-1002677824346',?,?)", enabled, status); err != nil {
						return err
					}
				}
				if tc.existing {
					if _, err := tx.Exec("INSERT INTO " + channel + " (id,tenant_id,telegram_chat_id) VALUES (201,23,'-1002677824346')"); err != nil {
						return err
					}
					var deleted any
					if tc.deleted {
						deleted = time.Now().Add(-time.Hour)
					}
					if _, err := tx.Exec("INSERT INTO "+relation+" (id,tenant_id,account_id,source_id,channel_id,collect_enabled,status,approval_status,disabled_reason,deleted_at) VALUES (301,23,469,101,201,1,?,?,'preserve-policy',?)", tc.status, tc.approval, deleted); err != nil {
						return err
					}
				}
				if tc.interleavingWinner {
					// Simulate a competing decision after the missing-row SELECT,
					// immediately before the outer INSERT's conflict check. This is
					// an interleaving simulation, not a multi-connection race test.
					for _, statement := range []string{
						`CREATE FUNCTION pg_temp.account_relation_insert_winner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.disabled_reason IS DISTINCT FROM 'winner' THEN
  INSERT INTO pg_temp.hg_youban_publish_collect_account_channel
   (id,tenant_id,account_id,channel_id,source_id,collect_enabled,status,approval_status,disabled_reason)
  VALUES (301,NEW.tenant_id,NEW.account_id,NEW.channel_id,NEW.source_id,0,'disabled','rejected','winner');
 END IF;
 RETURN NEW;
END $$`,
						`CREATE TRIGGER account_relation_winner BEFORE INSERT ON pg_temp.hg_youban_publish_collect_account_channel
 FOR EACH ROW EXECUTE FUNCTION pg_temp.account_relation_insert_winner()`,
					} {
						if _, err := tx.Exec(statement); err != nil {
							return err
						}
					}
				}
				delivery := &collectorin.CollectorDelivery{TenantID: 23, AccountID: 469, SourceID: 101, SourceChatID: "-1002677824346", SourceMessageID: 217520}
				s := &sSysPublish{}
				for attempt := 0; attempt < 2; attempt++ {
					allowed, err := s.ensureAccountCollectChannelRelation(ctx, delivery)
					if err != nil {
						return err
					}
					if allowed != tc.wantAllowed {
						return fmt.Errorf("attempt %d allowed=%v want=%v", attempt, allowed, tc.wantAllowed)
					}
				}
				// Raw TX reads include tombstones and prove no second row or resurrection.
				rows, err := tx.GetAll("SELECT id,status,approval_status,collect_enabled,disabled_reason,deleted_at FROM " + relation)
				if err != nil {
					return err
				}
				wantCount := 1
				if tc.sourceMissing {
					wantCount = 0
				}
				if len(rows) != wantCount {
					return fmt.Errorf("relation rows=%d want=%d", len(rows), wantCount)
				}
				if tc.interleavingWinner {
					r := rows[0]
					if r["id"].Int64() != 301 || r["status"].String() != "disabled" || r["approval_status"].String() != "rejected" || r["collect_enabled"].Int() != 0 || r["disabled_reason"].String() != "winner" || r["deleted_at"].GTime() != nil {
						return fmt.Errorf("conflict winner authorization was overwritten: %v", r)
					}
				} else if tc.existing {
					r := rows[0]
					if r["id"].Int64() != 301 || r["status"].String() != tc.status || r["approval_status"].String() != tc.approval || r["collect_enabled"].Int() != 1 || r["disabled_reason"].String() != "preserve-policy" {
						return fmt.Errorf("existing authorization was overwritten: %v", r)
					}
					if (r["deleted_at"].GTime() != nil) != tc.deleted {
						return fmt.Errorf("tombstone visibility changed")
					}
				} else if !tc.sourceMissing {
					wantEnabled := 0
					if tc.wantAllowed {
						wantEnabled = 1
					}
					if rows[0]["collect_enabled"].Int() != wantEnabled {
						return fmt.Errorf("new relation collect_enabled incorrect")
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Reuse installation column types/defaults and both real uniqueness fences.
// TEMP ON COMMIT DROP keeps every fixture and sequence on the transaction's
// connection; this fixture intentionally does not attempt multi-connection races.
func accountRelationLocalDDL(t *testing.T, install string) []string {
	t.Helper()
	ddl := []string{
		`CREATE TEMP TABLE hg_youban_publish_account(id bigint,tenant_id bigint,account_type text,status int,deleted_at timestamp) ON COMMIT DROP`,
		`CREATE TEMP TABLE hg_youban_publish_account_setting(account_id bigint,tenant_id bigint,collect_push_enabled int,deleted_at timestamp) ON COMMIT DROP`,
	}
	for _, table := range []string{"hg_youban_publish_collect_source", "hg_youban_publish_collect_channel", "hg_youban_publish_collect_account_channel"} {
		pattern := `(?s)CREATE TABLE IF NOT EXISTS "` + regexp.QuoteMeta(table) + `" \(.*?\n\);`
		statement := regexp.MustCompile(pattern).FindString(install)
		if statement == "" {
			t.Fatalf("installation DDL missing for %s", table)
		}
		statement = strings.Replace(statement, "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1)
		ddl = append(ddl, strings.TrimSuffix(statement, ";")+" ON COMMIT DROP")
	}
	for _, pattern := range []string{
		`(?m)^ALTER TABLE "hg_youban_publish_collect_source" ADD COLUMN IF NOT EXISTS "channel_id"[^\n]+`,
		`(?m)^CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_channel_tenant_chat"[^\n]+`,
		`(?m)^CREATE UNIQUE INDEX IF NOT EXISTS "uk_ybp_collect_account_channel_source"[^\n]+`,
	} {
		statement := regexp.MustCompile(pattern).FindString(install)
		if statement == "" {
			t.Fatalf("installation migration/index missing: %s", pattern)
		}
		ddl = append(ddl, statement)
	}
	return ddl
}
