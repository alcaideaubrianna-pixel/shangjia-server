package install

import (
	"context"
	"os"
	"strings"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestPostgresMenuUpgradeFitsOriginalSchema(t *testing.T) {
	link := os.Getenv("YOUBAN_RELAY_TEST_PG")
	if link == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	if !strings.Contains(link, "127.0.0.1") || !strings.HasSuffix(link, "/youban_relay_test") {
		t.Fatal("refusing non-test database")
	}
	gdb.SetConfigGroup("menu_sql_test", gdb.ConfigGroup{{Link: link}})
	db, err := gdb.Instance("menu_sql_test")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile("../../../storage/data/hotgo-pg.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(original), "CREATE TABLE IF NOT EXISTS hg_admin_menu (")
	if start < 0 {
		t.Fatal("original menu schema missing")
	}
	ddl := strings.Split(string(original)[start:], ";")[0]
	ddl = strings.Replace(ddl, "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1) + " ON COMMIT DROP"
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{ddl, "CREATE TEMP TABLE hg_admin_role(id bigint PRIMARY KEY) ON COMMIT DROP", "CREATE TEMP TABLE hg_admin_role_menu(role_id bigint,menu_id bigint) ON COMMIT DROP", `INSERT INTO hg_admin_menu(title,name)VALUES('插件','addons')`, `INSERT INTO hg_admin_role(id)VALUES(1),(2)`} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		content := readSqlFile("addons/youban_bot/resource/sql/menu.pgsql.sql")
		for i := 0; i < 2; i++ {
			for _, sql := range splitSql(content) {
				if strings.TrimSpace(sql) != "" {
					if _, err := tx.Exec(sql); err != nil {
						return err
					}
				}
			}
		}
		value, err := tx.GetValue(`SELECT COUNT(*) FROM hg_admin_menu WHERE name IN ('youbanBotConfig','youbanBotRelayConfig','youbanBotBroadcastRecords') AND type=3`)
		if err != nil {
			return err
		}
		if value.Int() != 3 {
			t.Fatal("hidden permission entries missing", value)
		}
		value, err = tx.GetValue(`SELECT COUNT(*) FROM hg_admin_menu WHERE name='youbanBotBroadcastRecords' AND (type<>3 OR path<>'' OR component<>'')`)
		if err != nil {
			return err
		}
		if value.Int() != 0 {
			t.Fatal("broadcast secondary menu recreated")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
