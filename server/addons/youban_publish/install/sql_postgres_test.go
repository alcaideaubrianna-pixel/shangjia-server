package install

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestRepeatedUpgradeDDLLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:55484")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	db := g.DB()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.Exec(ctx, "CREATE SCHEMA upgrade_ddl_test; CREATE TABLE upgrade_ddl_test.live(id int PRIMARY KEY, payload text); CREATE UNIQUE INDEX live_id ON upgrade_ddl_test.live(id); INSERT INTO upgrade_ddl_test.live VALUES(1,'old')"); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.Exec(context.Background(), "DROP SCHEMA upgrade_ddl_test CASCADE") }()
	path := filepath.Join(t.TempDir(), "upgrade.sql")
	if err := os.WriteFile(path, []byte(`CREATE UNIQUE INDEX IF NOT EXISTS live_id ON upgrade_ddl_test.live(id);
ALTER TABLE upgrade_ddl_test.live ADD COLUMN IF NOT EXISTS payload text;`), 0600); err != nil {
		t.Fatal(err)
	}
	// A live write is incompatible with CREATE INDEX's ShareLock and ALTER
	// TABLE's AccessExclusiveLock, even when IF NOT EXISTS has nothing to do.
	writer, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if _, err = writer.Exec("UPDATE upgrade_ddl_test.live SET payload='busy' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("SET LOCAL lock_timeout='200ms'"); err != nil {
			return err
		}
		if err := execSqlFile(ctx, tx, path); err != nil {
			return err
		}
		for _, stmt := range []string{`ALTER TABLE upgrade_ddl_test.live ADD COLUMN IF NOT EXISTS missing text`, `CREATE INDEX IF NOT EXISTS missing_idx ON upgrade_ddl_test.live(payload)`} {
			exists, err := postgresSchemaStatementExists(tx, stmt)
			if err != nil {
				return err
			}
			if exists {
				t.Fatalf("missing schema object treated as present: %s", stmt)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
