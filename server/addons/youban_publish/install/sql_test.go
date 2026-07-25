package install

import "testing"

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
