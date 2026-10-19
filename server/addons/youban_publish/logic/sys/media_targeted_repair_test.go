package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestBotMediaTargetedRepairLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_MEDIA_PROJECTION_LOCAL_PG") != "1" {
		t.Skip("requires isolated local PostgreSQL")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../../../tools/bot_media_repair/repair_cs12345_20261019.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	script := strings.Replace(string(raw), "\nBEGIN;\n", "\n", 1)
	script = strings.Replace(script, "\nCOMMIT;\n", "\n", 1)
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE TEMP TABLE hg_content_profile (id bigint,profile_no text,deleted_at timestamp,cover_media_id bigint,updated_at timestamp)",
			"CREATE TEMP TABLE hg_youban_publish_profile_state (profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp)",
			"CREATE TEMP TABLE hg_sys_attachment (id bigint,path text,size bigint,drive text,md5 text)",
			"CREATE TEMP TABLE hg_youban_publish_media (id bigint,profile_id bigint,tenant_id bigint,account_id bigint,media_type text,purpose text,sort_index int,storage_path text,original_storage_path text,poster_storage_path text,perceptual_hash text,size bigint,attachment_id bigint,original_attachment_id bigint,md5 text,deleted_at timestamp,updated_at timestamp)",
			"CREATE TEMP TABLE hg_content_media (id bigserial,profile_id bigint,source_asset_id bigint,media_type text,purpose text,sort_index int,original_storage_path text,display_storage_path text,preview_storage_path text,binary_md5 text,perceptual_hash text,process_status text,encrypt_status text,status int,deleted_at timestamp,created_at timestamp,updated_at timestamp)",
			"INSERT INTO hg_content_profile (id) VALUES(681235),(681236)",
			"INSERT INTO hg_youban_publish_profile_state VALUES(681235,80,620,NULL),(681236,80,620,NULL)",
			"INSERT INTO hg_sys_attachment SELECT n,'file/'||n,100,'cos','exact-md5-'||n FROM generate_series(5175590,5175611) n",
			"INSERT INTO hg_youban_publish_media (id,profile_id,tenant_id,account_id,media_type,purpose,sort_index,storage_path,original_storage_path,size) SELECT n,CASE WHEN n<=5175600 THEN 681235 ELSE 681236 END,80,620,CASE WHEN n IN (5175600,5175611) THEN 'video' ELSE 'image' END,CASE WHEN n IN (5175600,5175611) THEN 'verify' ELSE 'display' END,n%11,'file/'||n,'file/'||n,100 FROM generate_series(5175590,5175611) n",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		for i := 0; i < 2; i++ {
			if _, err := tx.Exec(script); err != nil {
				return err
			}
			row, err := tx.GetOne("SELECT count(*) n,count(*) FILTER(WHERE purpose='verify') v FROM hg_content_media WHERE deleted_at IS NULL")
			if err != nil {
				return err
			}
			if row["n"].Int() != 22 || row["v"].Int() != 2 {
				t.Fatalf("run=%d result=%v", i, row)
			}
		}
		// Ownership drift must abort rather than touching another user's data.
		if _, err := tx.Exec("SAVEPOINT wrong_owner"); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE hg_youban_publish_profile_state SET tenant_id=81"); err != nil {
			return err
		}
		if _, err := tx.Exec(script); err == nil {
			t.Fatal("expected ownership guard to reject repair")
		}
		_, err := tx.Exec("ROLLBACK TO SAVEPOINT wrong_owner")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
