package install

import (
	"slices"
	"strings"
	"testing"
)

func TestSplitSqlKeepsDollarQuotedBlock(t *testing.T) {
	statements := splitSql(`DO $$
BEGIN
  PERFORM 1;
END $$;
CREATE INDEX test_index ON test_table (id);`)
	if len(statements) != 2 {
		t.Fatalf("expected 2 SQL statements, got %d: %#v", len(statements), statements)
	}
	if statements[0] == "" || statements[1] == "" {
		t.Fatalf("expected non-empty SQL statements: %#v", statements)
	}
}

func TestSqlDollarQuoteTag(t *testing.T) {
	if got := sqlDollarQuoteTag("$migration$body"); got != "$migration$" {
		t.Fatalf("unexpected dollar quote tag: %q", got)
	}
	if got := sqlDollarQuoteTag("$1"); got != "" {
		t.Fatalf("numeric dollar prefix must not be treated as a quote: %q", got)
	}
}

func TestUpgradeSafeSqlIncludesCollectMediaRetryColumn(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		path  string
	}{
		{name: "mysql", files: mysqlUpgradeSafeSqlFiles, path: "addons/youban_publish/resource/sql/upgrade_safe.sql"},
		{name: "pgsql", files: pgsqlUpgradeSafeSqlFiles, path: "addons/youban_publish/resource/sql/upgrade_safe.pgsql.sql"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !slices.Contains(test.files, test.path) {
				t.Fatalf("upgrade SQL files do not contain %s: %#v", test.path, test.files)
			}
			if sql := readSqlFile(test.path); !strings.Contains(sql, "next_retry_at") {
				t.Fatalf("upgrade SQL does not add next_retry_at: %s", sql)
			}
		})
	}
}

func TestUpgradeSafeSqlIncludesProfileCycleDueIndex(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "mysql", path: "addons/youban_publish/resource/sql/upgrade_safe.sql"},
		{name: "pgsql", path: "addons/youban_publish/resource/sql/upgrade_safe.pgsql.sql"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if sql := readSqlFile(test.path); !strings.Contains(sql, "idx_ybp_tg_job_cycle_due") {
				t.Fatalf("upgrade SQL does not add profile cycle due index: %s", sql)
			}
		})
	}
}
